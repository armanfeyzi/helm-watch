package metrics

import (
	"sort"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

// ── Contract invariants ────────────────────────────────────────────────────
// These declarations are intentionally public within the test so that a
// reviewer can diff them against dashboards / alert rules.  Any change to
// label names or enum sets is a BREAKING schema change and must be
// intentional.

// requiredInfoLabels is the exact, sorted set of label names that every
// helm_chart_info time-series must carry.
var requiredInfoLabels = []string{
	"app", "chart", "current_version", "deployment_type",
	"latest_version", "namespace", "repo", "source_kind", "status",
}

// requiredSimpleLabels is the exact, sorted set of label names for the
// scalar gauges (helm_chart_outdated, helm_chart_version_lag,
// helm_chart_unknown).
var requiredSimpleLabels = []string{"app", "chart", "namespace"}

// validStatuses is the closed enum for the status label.
var validStatuses = map[string]bool{
	string(model.VersionStatusUpToDate): true,
	string(model.VersionStatusOutdated): true,
	string(model.VersionStatusUnknown):  true,
}

// validSourceKinds is the closed enum for the source_kind label.
var validSourceKinds = map[string]bool{
	"helm_repo":    true,
	"oci_registry": true,
	"git":          true,
	"unknown":      true,
}

// validDeploymentTypes is the closed enum for the deployment_type label.
var validDeploymentTypes = map[string]bool{
	string(model.DeploymentTypeArgoCD):    true,
	string(model.DeploymentTypeHelm):      true,
	string(model.DeploymentTypeTerraform): true,
	string(model.DeploymentTypeUnknown):   true,
}

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

// ── Contract tests ─────────────────────────────────────────────────────────

// TestChartInfoRequiredLabelSchema verifies that helm_chart_info carries
// exactly the labels declared in requiredInfoLabels — no more, no fewer.
// Adding, removing, or renaming a label is a breaking dashboard/alert change.
func TestChartInfoRequiredLabelSchema(t *testing.T) {
	reg := NewRegistry()
	m := NewChartMetrics(reg)
	engine := version.NewEngine()

	workloads := []model.WorkloadRecord{
		{ID: "w1", AppName: "prometheus", Namespace: "monitoring", DeploymentType: model.DeploymentTypeHelm},
	}
	chartRecords := []model.ChartRecord{
		{
			WorkloadID:     "w1",
			ChartName:      "prometheus",
			RepoURL:        "https://prometheus-community.github.io/helm-charts",
			SourceKind:     "helm_repo",
			CurrentVersion: "25.0.0",
			LatestVersion:  "25.1.0",
		},
	}

	m.Publish(workloads, chartRecords, engine)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	info := findFamily(t, families, "helm_chart_info")
	if len(info.GetMetric()) == 0 {
		t.Fatal("helm_chart_info: no time-series emitted")
	}
	for _, metric := range info.GetMetric() {
		got := sortedLabelNames(metric)
		if !equalStringSlices(got, requiredInfoLabels) {
			t.Fatalf("helm_chart_info label names = %v\nwant                                    %v",
				got, requiredInfoLabels)
		}
	}
}

// TestSimpleGaugeLabelSchema verifies that the three scalar gauges carry
// exactly the labels declared in requiredSimpleLabels.
func TestSimpleGaugeLabelSchema(t *testing.T) {
	simpleMetrics := []string{
		"helm_chart_outdated",
		"helm_chart_version_lag",
		"helm_chart_unknown",
	}

	reg := NewRegistry()
	m := NewChartMetrics(reg)
	engine := version.NewEngine()

	workloads := []model.WorkloadRecord{
		{ID: "w1", AppName: "grafana", Namespace: "monitoring", DeploymentType: model.DeploymentTypeHelm},
	}
	chartRecords := []model.ChartRecord{
		{WorkloadID: "w1", ChartName: "grafana", CurrentVersion: "10.0.0", LatestVersion: "11.0.0"},
	}

	m.Publish(workloads, chartRecords, engine)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, name := range simpleMetrics {
		family := findFamily(t, families, name)
		if len(family.GetMetric()) == 0 {
			t.Fatalf("%s: no time-series emitted", name)
		}
		for _, metric := range family.GetMetric() {
			got := sortedLabelNames(metric)
			if !equalStringSlices(got, requiredSimpleLabels) {
				t.Fatalf("%s label names = %v\nwant                  %v", name, got, requiredSimpleLabels)
			}
		}
	}
}

// TestStatusEnumDomain verifies that the status label on helm_chart_info is
// always a member of validStatuses regardless of version input.
func TestStatusEnumDomain(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
	}{
		{"outdated", "1.0.0", "2.0.0"},
		{"up_to_date", "2.0.0", "2.0.0"},
		{"unknown_latest", "1.0.0", "unknown"},
		{"empty_latest", "1.0.0", ""},
		{"both_empty", "", ""},
		{"non_semver_both", "abc", "xyz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			m := NewChartMetrics(reg)
			engine := version.NewEngine()

			workloads := []model.WorkloadRecord{
				{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: model.DeploymentTypeHelm},
			}
			chartRecords := []model.ChartRecord{
				{WorkloadID: "w1", ChartName: "chart", CurrentVersion: tc.current, LatestVersion: tc.latest},
			}

			m.Publish(workloads, chartRecords, engine)

			families, _ := reg.Gather()
			info := findFamily(t, families, "helm_chart_info")
			for _, metric := range info.GetMetric() {
				status := labelValue(metric, "status")
				if !validStatuses[status] {
					t.Fatalf("helm_chart_info status=%q is not in the valid enum set %v", status, validStatuses)
				}
			}
		})
	}
}

// TestSourceKindEnumDomain verifies that the source_kind label on
// helm_chart_info is always a member of validSourceKinds.
func TestSourceKindEnumDomain(t *testing.T) {
	cases := []struct {
		name       string
		sourceKind string
	}{
		{"helm_repo", "helm_repo"},
		{"oci_registry", "oci_registry"},
		{"git", "git"},
		{"unknown_explicit", "unknown"},
		{"empty_becomes_unknown", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			m := NewChartMetrics(reg)
			engine := version.NewEngine()

			workloads := []model.WorkloadRecord{
				{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: model.DeploymentTypeHelm},
			}
			chartRecords := []model.ChartRecord{
				{WorkloadID: "w1", ChartName: "chart", SourceKind: tc.sourceKind, CurrentVersion: "1.0.0", LatestVersion: "1.0.0"},
			}

			m.Publish(workloads, chartRecords, engine)

			families, _ := reg.Gather()
			info := findFamily(t, families, "helm_chart_info")
			for _, metric := range info.GetMetric() {
				sk := labelValue(metric, "source_kind")
				if !validSourceKinds[sk] {
					t.Fatalf("helm_chart_info source_kind=%q is not in the valid enum set %v", sk, validSourceKinds)
				}
			}
		})
	}
}

// TestDeploymentTypeEnumDomain verifies that the deployment_type label on
// helm_chart_info is always a member of validDeploymentTypes.
func TestDeploymentTypeEnumDomain(t *testing.T) {
	deployTypes := []model.DeploymentType{
		model.DeploymentTypeHelm,
		model.DeploymentTypeArgoCD,
		model.DeploymentTypeTerraform,
		model.DeploymentTypeUnknown,
	}

	for _, dt := range deployTypes {
		t.Run(string(dt), func(t *testing.T) {
			reg := NewRegistry()
			m := NewChartMetrics(reg)
			engine := version.NewEngine()

			workloads := []model.WorkloadRecord{
				{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: dt},
			}
			chartRecords := []model.ChartRecord{
				{WorkloadID: "w1", ChartName: "chart", CurrentVersion: "1.0.0", LatestVersion: "1.0.0"},
			}

			m.Publish(workloads, chartRecords, engine)

			families, _ := reg.Gather()
			info := findFamily(t, families, "helm_chart_info")
			for _, metric := range info.GetMetric() {
				got := labelValue(metric, "deployment_type")
				if !validDeploymentTypes[got] {
					t.Fatalf("helm_chart_info deployment_type=%q is not in the valid enum set %v", got, validDeploymentTypes)
				}
			}
		})
	}
}

// TestOutdatedGaugeBinaryValues verifies helm_chart_outdated is strictly 0 or
// 1 — never any other value — across all version comparison outcomes.
func TestOutdatedGaugeBinaryValues(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    float64
	}{
		{"outdated", "1.0.0", "2.0.0", 1},
		{"up_to_date", "2.0.0", "2.0.0", 0},
		{"newer_current", "3.0.0", "2.0.0", 0},
		{"unknown_latest", "1.0.0", "unknown", 0},
		{"empty_latest", "1.0.0", "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			m := NewChartMetrics(reg)
			engine := version.NewEngine()

			workloads := []model.WorkloadRecord{
				{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: model.DeploymentTypeHelm},
			}
			chartRecords := []model.ChartRecord{
				{WorkloadID: "w1", ChartName: "chart", CurrentVersion: tc.current, LatestVersion: tc.latest},
			}

			m.Publish(workloads, chartRecords, engine)

			families, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather: %v", err)
			}

			got := metricValue(t, families, "helm_chart_outdated", map[string]string{
				"app": "app", "namespace": "ns", "chart": "chart",
			})
			if got != tc.want {
				t.Fatalf("helm_chart_outdated=%v, want %v (current=%q latest=%q)", got, tc.want, tc.current, tc.latest)
			}
			if got != 0 && got != 1 {
				t.Fatalf("helm_chart_outdated=%v is not binary", got)
			}
		})
	}
}

// TestVersionLagNonNegative verifies helm_chart_version_lag is always >= 0.
func TestVersionLagNonNegative(t *testing.T) {
	cases := []struct {
		current string
		latest  string
	}{
		{"1.0.0", "2.0.0"},
		{"2.0.0", "2.0.0"},
		{"3.0.0", "2.0.0"},
		{"1.0.0", "unknown"},
		{"", ""},
	}

	for _, tc := range cases {
		reg := NewRegistry()
		m := NewChartMetrics(reg)
		engine := version.NewEngine()

		workloads := []model.WorkloadRecord{
			{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: model.DeploymentTypeHelm},
		}
		chartRecords := []model.ChartRecord{
			{WorkloadID: "w1", ChartName: "chart", CurrentVersion: tc.current, LatestVersion: tc.latest},
		}

		m.Publish(workloads, chartRecords, engine)

		families, err := reg.Gather()
		if err != nil {
			t.Fatalf("gather (current=%q latest=%q): %v", tc.current, tc.latest, err)
		}

		lag := metricValue(t, families, "helm_chart_version_lag", map[string]string{
			"app": "app", "namespace": "ns", "chart": "chart",
		})
		if lag < 0 {
			t.Fatalf("helm_chart_version_lag=%v < 0 (current=%q latest=%q)", lag, tc.current, tc.latest)
		}
	}
}

// TestGaugesConsistency verifies that helm_chart_outdated and helm_chart_unknown
// are mutually exclusive and consistent with the version comparison result.
func TestGaugesConsistency(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		wantOutdated    float64
		wantUnknown     float64
	}{
		{"outdated", "1.0.0", "2.0.0", 1, 0},
		{"up_to_date", "2.0.0", "2.0.0", 0, 0},
		{"unknown", "1.0.0", "unknown", 0, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			m := NewChartMetrics(reg)
			engine := version.NewEngine()

			workloads := []model.WorkloadRecord{
				{ID: "w1", AppName: "app", Namespace: "ns", DeploymentType: model.DeploymentTypeHelm},
			}
			chartRecords := []model.ChartRecord{
				{WorkloadID: "w1", ChartName: "chart", CurrentVersion: tc.current, LatestVersion: tc.latest},
			}

			m.Publish(workloads, chartRecords, engine)

			families, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather: %v", err)
			}

			lbls := map[string]string{"app": "app", "namespace": "ns", "chart": "chart"}

			outdated := metricValue(t, families, "helm_chart_outdated", lbls)
			if outdated != tc.wantOutdated {
				t.Errorf("helm_chart_outdated=%v, want %v", outdated, tc.wantOutdated)
			}

			unknown := metricValue(t, families, "helm_chart_unknown", lbls)
			if unknown != tc.wantUnknown {
				t.Errorf("helm_chart_unknown=%v, want %v", unknown, tc.wantUnknown)
			}

			if outdated == 1 && unknown == 1 {
				t.Error("helm_chart_outdated and helm_chart_unknown cannot both be 1")
			}
		})
	}
}

// ── Test helpers ───────────────────────────────────────────────────────────

func findFamily(t *testing.T, families []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	t.Fatalf("metric family %q not found", name)
	return nil
}

func sortedLabelNames(metric *dto.Metric) []string {
	names := make([]string, 0, len(metric.GetLabel()))
	for _, lp := range metric.GetLabel() {
		names = append(names, lp.GetName())
	}
	sort.Strings(names)
	return names
}

func labelValue(metric *dto.Metric, name string) string {
	for _, lp := range metric.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
