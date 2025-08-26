package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/afeyzirealyticsio/helm-watch/internal/config"
	"github.com/afeyzirealyticsio/helm-watch/internal/server"
)

func main() {
	cfg := config.FromEnv()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	srv := server.New(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		slog.Error("helm-watch exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("helm-watch stopped")
}
