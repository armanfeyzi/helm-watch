package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

type ChartMetrics struct {
	infoGauge     *prometheus.GaugeVec
	outdatedGauge *prometheus.GaugeVec
	lagGauge      *prometheus.GaugeVec

	reconcileDuration prometheus.Gauge
	reconcileErrors   prometheus.Counter
}

func NewChartMetrics(reg prometheus.Registerer) *ChartMetrics {
	m := &ChartMetrics{
		infoGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "helm_chart_info",
				Help: "Observed chart deployment and resolved version information.",
			},
			[]string{"app", "namespace", "chart", "repo", "source_kind", "current_version", "latest_version", "deployment_type"},
		),
		outdatedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "helm_chart_outdated",
				Help: "Whether a chart is outdated (1) or not (0).",
			},
			[]string{"app", "namespace", "chart"},
		),
		lagGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "helm_chart_version_lag",
				Help: "Numeric lag between current and latest chart versions.",
			},
			[]string{"app", "namespace", "chart"},
		),
		reconcileDuration: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "helm_watch_reconcile_duration_seconds",
				Help: "Duration of the latest metadata and metrics reconcile cycle.",
			},
		),
		reconcileErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "helm_watch_reconcile_errors_total",
				Help: "Total reconcile errors in metrics publication pipeline.",
			},
		),
	}

	reg.MustRegister(m.infoGauge, m.outdatedGauge, m.lagGauge, m.reconcileDuration, m.reconcileErrors)
	return m
}

func (m *ChartMetrics) Publish(workloads []model.WorkloadRecord, chartRecords []model.ChartRecord, engine *version.Engine) {
	m.infoGauge.Reset()
	m.outdatedGauge.Reset()
	m.lagGauge.Reset()

	workloadMap := make(map[string]model.WorkloadRecord, len(workloads))
	for _, w := range workloads {
		workloadMap[w.ID] = w
	}

	for _, rec := range chartRecords {
		workload, ok := workloadMap[rec.WorkloadID]
		if !ok {
			continue
		}

		m.infoGauge.WithLabelValues(
			workload.AppName,
			workload.Namespace,
			emptyToUnknown(rec.ChartName),
			emptyToUnknown(rec.RepoURL),
			emptyToUnknown(rec.SourceKind),
			emptyToUnknown(rec.CurrentVersion),
			emptyToUnknown(rec.LatestVersion),
			string(workload.DeploymentType),
		).Set(1)

		result := engine.Compare(rec.CurrentVersion, rec.LatestVersion)
		outdated := 0.0
		if result.Status == model.VersionStatusOutdated {
			outdated = 1
		}

		m.outdatedGauge.WithLabelValues(workload.AppName, workload.Namespace, emptyToUnknown(rec.ChartName)).Set(outdated)
		m.lagGauge.WithLabelValues(workload.AppName, workload.Namespace, emptyToUnknown(rec.ChartName)).Set(result.Lag)
	}
}

func (m *ChartMetrics) ObserveReconcileDuration(seconds float64) {
	m.reconcileDuration.Set(seconds)
}

func (m *ChartMetrics) IncReconcileError() {
	m.reconcileErrors.Inc()
}

func emptyToUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
