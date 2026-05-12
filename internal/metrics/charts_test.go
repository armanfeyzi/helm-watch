package metrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

func TestPublish_UnknownGaugeTracksUnknownStatus(t *testing.T) {
	reg := NewRegistry()
	m := NewChartMetrics(reg)
	engine := version.NewEngine()

	workloads := []model.WorkloadRecord{
		{
			ID:             "w1",
			AppName:        "vault",
			Namespace:      "security",
			DeploymentType: model.DeploymentTypeHelm,
		},
	}

	chartRecords := []model.ChartRecord{
		{
			WorkloadID:     "w1",
			ChartName:      "vault",
			CurrentVersion: "1.2.3",
			LatestVersion:  "unknown",
		},
	}

	m.Publish(workloads, chartRecords, engine)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	unknown := metricValue(t, families, "helm_chart_unknown", map[string]string{
		"app":       "vault",
		"namespace": "security",
		"chart":     "vault",
	})
	if unknown != 1 {
		t.Fatalf("helm_chart_unknown=%v, want 1", unknown)
	}

	outdated := metricValue(t, families, "helm_chart_outdated", map[string]string{
		"app":       "vault",
		"namespace": "security",
		"chart":     "vault",
	})
	if outdated != 0 {
		t.Fatalf("helm_chart_outdated=%v, want 0 for unknown version status", outdated)
	}
}

func metricValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric, labels) {
				return metric.GetGauge().GetValue()
			}
		}
		t.Fatalf("metric %q with labels %v not found", name, labels)
	}

	t.Fatalf("metric family %q not found", name)
	return 0
}

func labelsMatch(metric *dto.Metric, expected map[string]string) bool {
	if len(metric.GetLabel()) != len(expected) {
		return false
	}
	for _, label := range metric.GetLabel() {
		v, ok := expected[label.GetName()]
		if !ok || v != label.GetValue() {
			return false
		}
	}
	return true
}
