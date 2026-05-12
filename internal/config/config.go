package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/registryauth"
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
	RepoCacheTTL   time.Duration
	ResolveWorkers int
	RepoOverrides  map[string]string
	KubeconfigPath string
	LogLevel       slog.Level

	// RegistryCredentials supplies HTTP Basic auth for private Helm index and OCI
	// token endpoints, keyed by canonical registry hostname.
	RegistryCredentials map[string]registryauth.Credential
	// KubeClientQPS and KubeClientBurst tune client-side rate limiting for the
	// Kubernetes API (higher values reduce LIST throttling on large Argo CD fleets).
	KubeClientQPS   float32
	KubeClientBurst int
}

func FromEnv() Config {
	return Config{
		HTTPAddr:       getEnv("HELM_WATCH_HTTP_ADDR", defaultHTTPAddr),
		ReadTimeout:    getEnvDuration("HELM_WATCH_HTTP_READ_TIMEOUT", defaultReadTimeout),
		WriteTimeout:   getEnvDuration("HELM_WATCH_HTTP_WRITE_TIMEOUT", defaultWriteTimeout),
		ShutdownWindow: getEnvDuration("HELM_WATCH_SHUTDOWN_TIMEOUT", defaultShutdownWindow),
		ReconcileEvery: getEnvDuration("HELM_WATCH_RECONCILE_EVERY", 1*time.Hour),
		RepoCacheTTL:   getEnvDuration("HELM_WATCH_REPO_CACHE_TTL", 5*time.Minute),
		ResolveWorkers: getEnvInt("HELM_WATCH_RESOLVE_WORKERS", 8),
		RepoOverrides:  getEnvRepoOverrides("HELM_WATCH_REPO_OVERRIDES"),
		KubeconfigPath: os.Getenv("HELM_WATCH_KUBECONFIG"),
		LogLevel:       getEnvLogLevel("HELM_WATCH_LOG_LEVEL", slog.LevelInfo),

		RegistryCredentials: loadRegistryCredentialsFromEnv(),
		KubeClientQPS:       getEnvFloat32("HELM_WATCH_KUBE_CLIENT_QPS", 40),
		KubeClientBurst:     getEnvIntMin("HELM_WATCH_KUBE_CLIENT_BURST", 80, 1),
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

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func getEnvIntMin(key string, fallback, min int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return fallback
	}
	return n
}

func getEnvFloat32(key string, fallback float32) float32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil || f <= 0 {
		return fallback
	}
	return float32(f)
}

func loadRegistryCredentialsFromEnv() map[string]registryauth.Credential {
	filePath := strings.TrimSpace(os.Getenv("HELM_WATCH_REGISTRY_CREDENTIALS_FILE"))
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Warn("registry credentials file not loaded", "path", filePath, "error", err)
			return nil
		}
		m, err := registryauth.ParseCredentialsJSON(data)
		if err != nil {
			slog.Warn("registry credentials file invalid", "path", filePath, "error", err)
			return nil
		}
		return m
	}

	raw := strings.TrimSpace(os.Getenv("HELM_WATCH_REGISTRY_CREDENTIALS"))
	if raw == "" {
		return nil
	}
	m, err := registryauth.ParseCredentialsJSON([]byte(raw))
	if err != nil {
		slog.Warn("HELM_WATCH_REGISTRY_CREDENTIALS not loaded", "error", err)
		return nil
	}
	return m
}

func getEnvRepoOverrides(key string) map[string]string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return map[string]string{}
	}

	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		chart := strings.TrimSpace(parts[0])
		repo := strings.TrimSpace(parts[1])
		if chart == "" || repo == "" {
			continue
		}
		out[strings.ToLower(chart)] = repo
	}

	return out
}
