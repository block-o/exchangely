package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/block-o/exchangely/backend/internal/app"
	"github.com/block-o/exchangely/backend/internal/config"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := application.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
