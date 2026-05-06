package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/afeyzirealyticsio/helm-watch/internal/config"
	"github.com/afeyzirealyticsio/helm-watch/internal/discovery"
	"github.com/afeyzirealyticsio/helm-watch/internal/kube"
	"github.com/afeyzirealyticsio/helm-watch/internal/metrics"
)

type Server struct {
	cfg              config.Config
	registry         *metrics.Registry
	httpSrv          *http.Server
	discoveryManager *discovery.Manager
}

func New(cfg config.Config) (*Server, error) {
	reg := metrics.NewRegistry()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg.Gatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpSrv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	clients, err := kube.NewClients(cfg.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("initialize kubernetes clients: %w", err)
	}

	composite := discovery.NewComposite(
		discovery.NewArgoCDApplicationDiscoverer(clients.Dynamic),
		discovery.NewHelmReleaseDiscoverer(clients.Kubernetes),
	)
	discoveryManager := discovery.NewManager(composite, cfg.ReconcileEvery)

	return &Server{
		cfg:              cfg,
		registry:         reg,
		httpSrv:          httpSrv,
		discoveryManager: discoveryManager,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go s.discoveryManager.Run(ctx)

	go func() {
		slog.Info("starting HTTP server", "addr", s.cfg.HTTPAddr)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server failed: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownWindow)
		defer cancel()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown failed: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
