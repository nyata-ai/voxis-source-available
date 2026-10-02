package config

import (
	"io"
	"log/slog"
	"os"
)

// SetupLogger creates a configured slog logger based on environment
func SetupLogger(env string) *slog.Logger {
	return SetupLoggerWithWriter(env, os.Stdout)
}

// SetupLoggerWithWriter creates a logger with custom writer (useful for testing)
func SetupLoggerWithWriter(env string, w io.Writer) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		AddSource: env != "production",
		Level:     getLogLevel(env),
	}

	switch env {
	case "production":
		handler = slog.NewJSONHandler(w, opts)
	case "test":
		handler = slog.NewTextHandler(io.Discard, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler).With(
		"service", "voxis-api",
		"instance", InstanceID,
	)
}

func getLogLevel(env string) slog.Level {
	switch env {
	case "production":
		return slog.LevelInfo
	case "debug":
		return slog.LevelDebug
	default:
		return slog.LevelDebug
	}
}
