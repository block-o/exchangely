package telemetry

import (
	"log/slog"
	"os"
)

func ConfigureLogger(level slog.Leveler) *slog.Logger {
	if level == nil {
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	return logger
}
