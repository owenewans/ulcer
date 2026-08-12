package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type Server struct {
	controlv1.UnimplementedControlPlaneServer
	store  *store.Store
	events *events.Bus
	hub    *Hub
}

func NewServer(store *store.Store, events *events.Bus, hub *Hub) *Server {
	return &Server{store: store, events: events, hub: hub}
}

func (s *Server) Connect(stream grpc.BidiStreamingServer[controlv1.InstanceMessage, controlv1.HostMessage]) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "receive hello: %v", err)
	}
	hello := first.GetHello()
	if hello == nil || hello.InstanceId == "" {
		return status.Error(codes.InvalidArgument, "first message must be instance hello")
	}
	if err := authorizeInstance(stream.Context(), hello.InstanceId); err != nil {
		return err
	}
	instance, err := s.store.UpdateInstance(hello.InstanceId, func(instance *model.Instance) error {
		now := time.Now().UTC()
		instance.LastSeenAt = &now
		instance.AgentVersion = hello.AgentVersion
		instance.Capabilities = hello.Capabilities
		if instance.Phase == "enrolled" || instance.Phase == "offline" {
			instance.Phase = "connected"
		}
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		return status.Error(codes.PermissionDenied, "instance is not enrolled")
	}
	if err != nil {
		return status.Errorf(codes.Internal, "load instance: %v", err)
	}

	outbound, detach := s.hub.Attach(hello.InstanceId)
	s.publish("instance.connected", instance.ID, map[string]any{"name": instance.Name})
	defer func() {
		if detach() {
			s.markOffline(instance.ID)
		}
	}()

	if instance.DesiredGeneration > 0 {
		if err := stream.Send(desiredMessage(instance)); err != nil {
			return err
		}
	}

	recv := make(chan *controlv1.InstanceMessage)
	recvErr := make(chan error, 1)
	go func() {
		defer close(recv)
		for {
			message, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			select {
			case recv <- message:
			case <-stream.Context().Done():
				return
			}
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case err := <-recvErr:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case message, ok := <-recv:
			if !ok {
				return nil
			}
			if err := s.handleMessage(hello.InstanceId, message, stream); err != nil {
				return err
			}
		case message, ok := <-outbound:
			if !ok {
				return status.Error(codes.Aborted, "connection replaced")
			}
			if err := stream.Send(message); err != nil {
				return err
			}
		}
	}
}

func (s *Server) handleMessage(instanceID string, message *controlv1.InstanceMessage, stream grpc.BidiStreamingServer[controlv1.InstanceMessage, controlv1.HostMessage]) error {
	switch body := message.Body.(type) {
	case *controlv1.InstanceMessage_Status:
		statusMessage := body.Status
		if statusMessage.InstanceId != instanceID {
			return status.Error(codes.PermissionDenied, "status identity mismatch")
		}
		updated, err := s.store.UpdateInstance(instanceID, func(instance *model.Instance) error {
			if statusMessage.AppliedGeneration > instance.DesiredGeneration {
				return fmt.Errorf("applied generation exceeds desired generation")
			}
			if statusMessage.AppliedGeneration == instance.DesiredGeneration &&
				statusMessage.AppliedGeneration > 0 && statusMessage.SpecDigest != instance.DesiredDigest {
				return fmt.Errorf("digest differs at generation %d", statusMessage.AppliedGeneration)
			}
			now := time.Now().UTC()
			instance.LastSeenAt = &now
			instance.AppliedGeneration = statusMessage.AppliedGeneration
			instance.AppliedDigest = statusMessage.SpecDigest
			instance.Phase = statusMessage.Phase
			instance.Reason = statusMessage.Reason
			return nil
		})
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "apply status: %v", err)
		}
		s.publish("instance.status", instanceID, map[string]any{"phase": updated.Phase, "generation": updated.AppliedGeneration})
	case *controlv1.InstanceMessage_Meters:
		meters := body.Meters
		if meters.InstanceId != instanceID {
			return status.Error(codes.PermissionDenied, "meter identity mismatch")
		}
		if len(meters.Deltas) > 0 && (meters.FirstSequence == 0 || meters.Deltas[0] == nil || meters.FirstSequence != meters.Deltas[0].Sequence) {
			return status.Error(codes.InvalidArgument, "meter batch first sequence mismatch")
		}
		ack, err := s.store.ApplyMeters(instanceID, meters.Deltas)
		if err != nil {
			return status.Errorf(codes.Internal, "apply meters: %v", err)
		}
		if err := stream.Send(&controlv1.HostMessage{Body: &controlv1.HostMessage_MeterAck{MeterAck: &controlv1.MeterAck{
			InstanceId:         instanceID,
			ContiguousSequence: ack,
		}}}); err != nil {
			return err
		}
		s.publish("traffic.updated", instanceID, map[string]any{"sequence": ack})
	case *controlv1.InstanceMessage_Hello:
		return status.Error(codes.InvalidArgument, "hello can only be sent once")
	default:
		return status.Error(codes.InvalidArgument, "empty instance message")
	}
	return nil
}

func (s *Server) markOffline(id string) {
	_, _ = s.store.UpdateInstance(id, func(instance *model.Instance) error {
		instance.Phase = "offline"
		return nil
	})
	s.publish("instance.disconnected", id, nil)
}

func (s *Server) publish(eventType, resourceID string, data map[string]any) {
	_, _ = s.events.Publish(model.Event{Type: eventType, ResourceID: resourceID, Data: data})
}

func desiredMessage(instance model.Instance) *controlv1.HostMessage {
	return &controlv1.HostMessage{Body: &controlv1.HostMessage_Desired{Desired: &controlv1.DesiredState{
		InstanceId:    instance.ID,
		Generation:    instance.DesiredGeneration,
		SpecDigest:    instance.DesiredDigest,
		CanonicalSpec: instance.DesiredSpec,
	}}}
}

func authorizeInstance(ctx context.Context, id string) error {
	connection, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing peer")
	}
	tlsInfo, ok := connection.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return status.Error(codes.Unauthenticated, "verified client certificate required")
	}
	certificate := tlsInfo.State.VerifiedChains[0][0]
	want := "spiffe://ulcer/instance/" + id
	for _, uri := range certificate.URIs {
		if strings.EqualFold(uri.String(), want) {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, "certificate is not bound to instance")
}
