package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	defaultHTTPAddr       = ":8080"
	defaultReadTimeout    = 10 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultShutdownWindow = 10 * time.Second
)

type Config struct {
	HTTPAddr       string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	ShutdownWindow time.Duration
	ReconcileEvery time.Duration
	KubeconfigPath string
	LogLevel       slog.Level
}

func FromEnv() Config {
	return Config{
		HTTPAddr:       getEnv("HELM_WATCH_HTTP_ADDR", defaultHTTPAddr),
		ReadTimeout:    getEnvDuration("HELM_WATCH_HTTP_READ_TIMEOUT", defaultReadTimeout),
		WriteTimeout:   getEnvDuration("HELM_WATCH_HTTP_WRITE_TIMEOUT", defaultWriteTimeout),
		ShutdownWindow: getEnvDuration("HELM_WATCH_SHUTDOWN_TIMEOUT", defaultShutdownWindow),
		ReconcileEvery: getEnvDuration("HELM_WATCH_RECONCILE_EVERY", 30*time.Second),
		KubeconfigPath: os.Getenv("HELM_WATCH_KUBECONFIG"),
		LogLevel:       getEnvLogLevel("HELM_WATCH_LOG_LEVEL", slog.LevelInfo),
	}
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}

	return d
}

func getEnvLogLevel(key string, fallback slog.Level) slog.Level {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	if i, err := strconv.Atoi(v); err == nil {
		return slog.Level(i)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(v)); err != nil {
		return fallback
	}
	return level
}
