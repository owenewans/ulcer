package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/api"
	"github.com/owenewans/ulcer/internal/auth"
	"github.com/owenewans/ulcer/internal/config"
	"github.com/owenewans/ulcer/internal/control"
	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/pki"
	"github.com/owenewans/ulcer/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("host stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	configuration := config.HostFromEnv()
	if err := os.MkdirAll(configuration.DataDir, 0o700); err != nil {
		return err
	}
	database, err := store.Open(filepath.Join(configuration.DataDir, "badger"), false)
	if err != nil {
		return err
	}
	defer database.Close()

	authService, generated, err := auth.New(database, configuration.DataDir, configuration.SetupToken)
	if err != nil {
		return err
	}
	if generated {
		logger.Warn("operator setup token generated", "path", filepath.Join(configuration.DataDir, "setup.token"))
	}
	authority, serverBundle, err := pki.Ensure(configuration.DataDir, configuration.PublicName)
	if err != nil {
		return err
	}
	certificate, err := tls.X509KeyPair([]byte(serverBundle.CertificatePEM), []byte(serverBundle.PrivateKeyPEM))
	if err != nil {
		return err
	}

	eventBus := events.New(database)
	hub := control.NewHub()
	controlServer := control.NewServer(database, eventBus, hub)
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    authority.ClientPool(),
	})))
	controlv1.RegisterControlPlaneServer(grpcServer, controlServer)
	grpcListener, err := net.Listen("tcp", configuration.GRPCAddr)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              configuration.HTTPAddr,
		Handler:           api.New(database, authService, authority, eventBus, hub, logger, configuration),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 2)
	go func() {
		logger.Info("REST API listening", "address", configuration.HTTPAddr)
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- err
		}
	}()
	go func() {
		logger.Info("mTLS control plane listening", "address", configuration.GRPCAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			errorsChannel <- err
		}
	}()

	select {
	case err := <-errorsChannel:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		grpcServer.GracefulStop()
		return httpServer.Shutdown(shutdown)
	}
}
