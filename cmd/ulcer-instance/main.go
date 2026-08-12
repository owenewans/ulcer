package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/owenewans/ulcer/internal/config"
	instanceagent "github.com/owenewans/ulcer/internal/instance"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	configuration, err := config.InstanceFromEnv()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	client, err := instanceagent.New(configuration, logger)
	if err != nil {
		logger.Error("create instance", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := client.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("instance stopped", "error", err)
		os.Exit(1)
	}
}
