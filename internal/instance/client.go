package instance

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/buildinfo"
	"github.com/owenewans/ulcer/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrIdentityRejected = errors.New("instance identity rejected")

type Client struct {
	config config.Instance
	logger *slog.Logger
	state  *stateStore
}

func New(config config.Instance, logger *slog.Logger) (*Client, error) {
	state, err := newStateStore()
	if err != nil {
		return nil, err
	}
	return &Client{config: config, logger: logger, state: state}, nil
}

func (c *Client) Close() error {
	return c.state.close()
}

func (c *Client) Run(ctx context.Context) error {
	tlsConfig, err := c.tlsConfig()
	if err != nil {
		return err
	}
	backoff := time.Second
	for {
		if err := c.connect(ctx, tlsConfig); identityRejected(err) {
			return fmt.Errorf("%w: %v", ErrIdentityRejected, err)
		} else if err != nil && !errors.Is(err, context.Canceled) {
			c.logger.Warn("control stream disconnected", "error", err, "retry", backoff)
		} else if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func identityRejected(err error) bool {
	code := status.Code(err)
	return code == codes.Unauthenticated || code == codes.PermissionDenied
}

func (c *Client) connect(ctx context.Context, tlsConfig *tls.Config) error {
	connection, err := grpc.NewClient(c.config.Host, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return err
	}
	defer connection.Close()
	stream, err := controlv1.NewControlPlaneClient(connection).Connect(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&controlv1.InstanceMessage{Body: &controlv1.InstanceMessage_Hello{Hello: &controlv1.InstanceHello{
		InstanceId: c.config.ID, Name: c.config.Name, AgentVersion: buildinfo.Current().Version, Capabilities: []string{"desired-state-v1"},
	}}}); err != nil {
		return err
	}
	c.logger.Info("connected to host", "address", c.config.Host, "instance", c.config.ID)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	received := make(chan *controlv1.HostMessage)
	receiveErrors := make(chan error, 1)
	go func() {
		defer close(received)
		for {
			message, err := stream.Recv()
			if err != nil {
				receiveErrors <- err
				return
			}
			select {
			case received <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := c.sendStatus(stream); err != nil {
				return err
			}
		case err := <-receiveErrors:
			return receiveError(err)
		case message, ok := <-received:
			if !ok {
				// Recv reports its error before closing received. Prefer that status
				// so a revoked identity cannot be mistaken for a retryable EOF.
				select {
				case err := <-receiveErrors:
					return receiveError(err)
				case <-ctx.Done():
					return ctx.Err()
				default:
					return errors.New("host closed control stream")
				}
			}
			switch body := message.Body.(type) {
			case *controlv1.HostMessage_Desired:
				if body.Desired.InstanceId != c.config.ID {
					return errors.New("desired state identity mismatch")
				}
				state, err := c.state.apply(body.Desired)
				if err != nil {
					return fmt.Errorf("apply desired state: %w", err)
				}
				c.logger.Info("desired state reconciled", "generation", state.AppliedGeneration, "phase", state.Phase)
				if err := c.sendStatus(stream); err != nil {
					return err
				}
			case *controlv1.HostMessage_MeterAck:
				c.logger.Debug("meter batch acknowledged", "sequence", body.MeterAck.ContiguousSequence)
			case *controlv1.HostMessage_Notice:
				c.logger.Info("host notice", "code", body.Notice.Code, "message", body.Notice.Message)
			}
		}
	}
}

func receiveError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("host closed control stream")
	}
	return err
}

func (c *Client) sendStatus(stream grpc.BidiStreamingClient[controlv1.InstanceMessage, controlv1.HostMessage]) error {
	state, err := c.state.load()
	if err != nil {
		return err
	}
	return stream.Send(&controlv1.InstanceMessage{Body: &controlv1.InstanceMessage_Status{Status: &controlv1.InstanceStatus{
		InstanceId: c.config.ID, AppliedGeneration: state.AppliedGeneration, SpecDigest: state.SpecDigest,
		Phase: state.Phase, Reason: state.Reason, ObservedAt: timestamppb.Now(),
	}}})
}

func (c *Client) tlsConfig() (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(c.config.CertFile, c.config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load instance certificate: %w", err)
	}
	caPEM, err := os.ReadFile(c.config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("load host authority: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("host authority contains no certificate")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		RootCAs:      pool,
		ServerName:   c.config.ServerName,
	}, nil
}
