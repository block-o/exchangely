package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/block-o/exchangely/backend/internal/app"
	"github.com/block-o/exchangely/backend/internal/config"
	"github.com/block-o/exchangely/backend/internal/telemetry"
)

func main() {
	cfg := config.Load()
	logger := telemetry.ConfigureLogger(telemetry.ParseLevel(cfg.LogLevel))
	logger.Info("starting exchangely backend process")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg)
	if err != nil {
		logger.Error("application startup failed", "error", err)
		os.Exit(1)
	}
	if err := application.Run(ctx); err != nil {
		logger.Error("application terminated", "error", err)
		os.Exit(1)
	}

	logger.Info("application exited cleanly")
}
