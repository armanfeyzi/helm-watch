package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/afeyzirealyticsio/helm-watch/internal/catalog"
	"github.com/afeyzirealyticsio/helm-watch/internal/config"
	"github.com/afeyzirealyticsio/helm-watch/internal/discovery"
	"github.com/afeyzirealyticsio/helm-watch/internal/kube"
	"github.com/afeyzirealyticsio/helm-watch/internal/metrics"
	"github.com/afeyzirealyticsio/helm-watch/internal/resolver"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

type Server struct {
	cfg              config.Config
	registry         *metrics.Registry
	chartMetrics     *metrics.ChartMetrics
	httpSrv          *http.Server
	discoveryManager *discovery.Manager
	catalogBuilder   *catalog.Builder
	versionEngine    *version.Engine
}

func New(cfg config.Config) (*Server, error) {
	reg := metrics.NewRegistry()
	chartMetrics := metrics.NewChartMetrics(reg.Registerer)

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
	versionEngine := version.NewEngine()
	repoResolver := resolver.NewRepositoryResolver(nil, cfg.RepoCacheTTL)
	catalogBuilder := catalog.NewBuilder(clients.Dynamic, clients.Kubernetes, repoResolver, versionEngine)

	return &Server{
		cfg:              cfg,
		registry:         reg,
		chartMetrics:     chartMetrics,
		httpSrv:          httpSrv,
		discoveryManager: discoveryManager,
		catalogBuilder:   catalogBuilder,
		versionEngine:    versionEngine,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go s.discoveryManager.Run(ctx)
	go s.runMetricsPipeline(ctx)

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

func (s *Server) runMetricsPipeline(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.ReconcileEvery)
	defer ticker.Stop()

	s.publishMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.publishMetrics(ctx)
		}
	}
}

func (s *Server) publishMetrics(ctx context.Context) {
	start := time.Now()
	workloads := s.discoveryManager.Snapshot()
	records := s.catalogBuilder.Build(ctx, workloads)
	s.chartMetrics.Publish(workloads, records, s.versionEngine)
	s.chartMetrics.ObserveReconcileDuration(time.Since(start).Seconds())

	slog.Info("metrics reconcile completed", "workload_count", len(workloads), "chart_records", len(records), "duration_ms", time.Since(start).Milliseconds())
}
