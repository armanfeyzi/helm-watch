package catalog_test

// Regression coverage for the Argo CD discovery -> catalog enrichment chain.
//
// These tests exist because the namespace contract between discovery and
// catalog is easy to get wrong: an Argo CD `Application` CR lives in the
// Argo CD installation namespace (e.g. `argocd`), while the workload it
// renders is deployed to `spec.destination.namespace` (e.g. `monitoring`).
// Helm Watch must:
//
//   - Expose the *destination* namespace in metrics labels.
//   - Use the *Application CR* namespace to re-fetch the object during
//     catalog enrichment.
//
// A previous regression mixed those two up and broke enrichment for every
// Argo CD application whose destination namespace differed from the
// Application CR's namespace. The tests below fail loudly if either side
// of that contract is reversed again.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/afeyzirealyticsio/helm-watch/internal/catalog"
	"github.com/afeyzirealyticsio/helm-watch/internal/discovery"
	"github.com/afeyzirealyticsio/helm-watch/internal/metrics"
	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/resolver"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

const (
	argoCDNamespace     = "argocd"
	monitoringNamespace = "monitoring"
	cacheNamespace      = "cache"
	alloyCurrentVersion = "1.6.0"
	alloyLatestVersion  = "1.8.2"
	redisCurrentVersion = "20.0.0"
	redisLatestVersion  = "20.5.0"
)

var argoCDApplicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// fakeArgoCDIndex serves a minimal Helm `index.yaml` so the resolver can
// produce a deterministic latest version without reaching the public
// internet.
const fakeArgoCDIndex = `
apiVersion: v1
entries:
  alloy:
    - version: 1.8.2
    - version: 1.6.0
  redis:
    - version: 20.5.0
    - version: 20.0.0
`

// newFakeHelmRepo spins up a httptest server that returns the same
// `index.yaml` regardless of the request path so it can stand in for any
// Helm repo URL the test wires through the resolver.
func newFakeHelmRepo(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fakeArgoCDIndex))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newArgoApp builds an unstructured Argo CD Application object sitting in
// the Argo CD CR namespace but targeting a different destination namespace.
// Setting `multiSource` to true forces the helm entry into `spec.sources[]`
// alongside a non-Helm git source, exercising the multi-source path.
func newArgoApp(name, destNamespace, repoURL, chart, targetRevision string, multiSource bool) *unstructured.Unstructured {
	source := map[string]any{
		"repoURL":        repoURL,
		"chart":          chart,
		"targetRevision": targetRevision,
	}

	spec := map[string]any{
		"destination": map[string]any{
			"namespace": destNamespace,
			"server":    "https://kubernetes.default.svc",
		},
	}

	if multiSource {
		spec["sources"] = []any{
			map[string]any{
				"repoURL": "https://git.example.com/values-repo.git",
				"path":    "envs/prod",
			},
			source,
		}
	} else {
		spec["source"] = source
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]any{
				"name":      name,
				"namespace": argoCDNamespace,
			},
			"spec": spec,
		},
	}
}

func newDynamicClientWithApps(apps ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		argoCDApplicationGVR.GroupVersion().WithKind("Application"),
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		argoCDApplicationGVR.GroupVersion().WithKind("ApplicationList"),
		&unstructured.UnstructuredList{},
	)

	objs := make([]runtime.Object, 0, len(apps))
	for _, app := range apps {
		objs = append(objs, app)
	}

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			argoCDApplicationGVR: "ApplicationList",
		},
		objs...,
	)
}

func newBuilder(t *testing.T, dyn *dynamicfake.FakeDynamicClient) *catalog.Builder {
	t.Helper()
	repoResolver := resolver.NewRepositoryResolver(nil, time.Minute)
	ociResolver := resolver.NewOCIResolver(nil, time.Minute)
	return catalog.NewBuilder(
		dyn,
		kubefake.NewSimpleClientset(),
		repoResolver,
		ociResolver,
		version.NewEngine(),
		1,
		nil,
	)
}

// TestArgoDiscoveryToCatalog_SingleSource exercises the canonical
// single-source case: an Argo CD Application using `spec.source` with a
// helm chart entry. We verify both the namespace contract and the chart
// values that flow downstream into metrics.
func TestArgoDiscoveryToCatalog_SingleSource(t *testing.T) {
	srv := newFakeHelmRepo(t)
	app := newArgoApp("alloy", monitoringNamespace, srv.URL, "alloy", alloyCurrentVersion, false)
	dyn := newDynamicClientWithApps(app)

	disc := discovery.NewArgoCDApplicationDiscoverer(dyn)
	workloads, err := disc.Discover(context.Background())
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if len(workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(workloads))
	}

	w := workloads[0]
	if w.Namespace != monitoringNamespace {
		t.Fatalf("WorkloadRecord.Namespace = %q, want %q (destination namespace must be exposed for metrics)", w.Namespace, monitoringNamespace)
	}
	if w.SourceNamespace != argoCDNamespace {
		t.Fatalf("WorkloadRecord.SourceNamespace = %q, want %q (Application CR namespace must be retained for re-lookup)", w.SourceNamespace, argoCDNamespace)
	}

	builder := newBuilder(t, dyn)
	records := builder.Build(context.Background(), workloads)
	if len(records) != 1 {
		t.Fatalf("expected 1 chart record, got %d", len(records))
	}

	rec := records[0]
	if rec.ResolutionError != "" {
		t.Fatalf("unexpected resolution error: %q", rec.ResolutionError)
	}
	if rec.ChartName != "alloy" {
		t.Fatalf("ChartName = %q, want %q", rec.ChartName, "alloy")
	}
	if rec.RepoURL != srv.URL {
		t.Fatalf("RepoURL = %q, want %q", rec.RepoURL, srv.URL)
	}
	if rec.SourceKind != "helm_repo" {
		t.Fatalf("SourceKind = %q, want %q", rec.SourceKind, "helm_repo")
	}
	if rec.CurrentVersion != alloyCurrentVersion {
		t.Fatalf("CurrentVersion = %q, want %q", rec.CurrentVersion, alloyCurrentVersion)
	}
	if rec.LatestVersion != alloyLatestVersion {
		t.Fatalf("LatestVersion = %q, want %q", rec.LatestVersion, alloyLatestVersion)
	}
	if rec.Status != model.VersionStatusOutdated {
		t.Fatalf("Status = %q, want %q", rec.Status, model.VersionStatusOutdated)
	}
}

// TestArgoDiscoveryToCatalog_MultiSource exercises the `spec.sources[]`
// path where the helm chart is one entry among several. The same namespace
// contract must hold and the helm entry must still be picked out for
// enrichment.
func TestArgoDiscoveryToCatalog_MultiSource(t *testing.T) {
	srv := newFakeHelmRepo(t)
	app := newArgoApp("redis", cacheNamespace, srv.URL, "redis", redisCurrentVersion, true)
	dyn := newDynamicClientWithApps(app)

	disc := discovery.NewArgoCDApplicationDiscoverer(dyn)
	workloads, err := disc.Discover(context.Background())
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if len(workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(workloads))
	}

	w := workloads[0]
	if w.Namespace != cacheNamespace {
		t.Fatalf("WorkloadRecord.Namespace = %q, want %q", w.Namespace, cacheNamespace)
	}
	if w.SourceNamespace != argoCDNamespace {
		t.Fatalf("WorkloadRecord.SourceNamespace = %q, want %q", w.SourceNamespace, argoCDNamespace)
	}

	builder := newBuilder(t, dyn)
	records := builder.Build(context.Background(), workloads)
	if len(records) != 1 {
		t.Fatalf("expected 1 chart record, got %d", len(records))
	}

	rec := records[0]
	if rec.ResolutionError != "" {
		t.Fatalf("unexpected resolution error: %q", rec.ResolutionError)
	}
	if rec.ChartName != "redis" {
		t.Fatalf("ChartName = %q, want %q", rec.ChartName, "redis")
	}
	if rec.RepoURL != srv.URL {
		t.Fatalf("RepoURL = %q, want %q", rec.RepoURL, srv.URL)
	}
	if rec.CurrentVersion != redisCurrentVersion {
		t.Fatalf("CurrentVersion = %q, want %q", rec.CurrentVersion, redisCurrentVersion)
	}
	if rec.LatestVersion != redisLatestVersion {
		t.Fatalf("LatestVersion = %q, want %q", rec.LatestVersion, redisLatestVersion)
	}
	if rec.Status != model.VersionStatusOutdated {
		t.Fatalf("Status = %q, want %q", rec.Status, model.VersionStatusOutdated)
	}
}

// TestArgoCatalog_RegressionNamespaceSwap reproduces the historical
// regression: if SourceNamespace points at the destination namespace
// (i.e. the bug), the catalog builder will look for the Application CR in
// the wrong namespace and enrichment must fail with a clear lookup error.
// This guards the mapping from drifting again.
func TestArgoCatalog_RegressionNamespaceSwap(t *testing.T) {
	srv := newFakeHelmRepo(t)
	app := newArgoApp("alloy", monitoringNamespace, srv.URL, "alloy", alloyCurrentVersion, false)
	dyn := newDynamicClientWithApps(app)

	disc := discovery.NewArgoCDApplicationDiscoverer(dyn)
	workloads, err := disc.Discover(context.Background())
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if len(workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(workloads))
	}

	// Simulate the bug: collapse SourceNamespace onto the destination namespace.
	buggy := workloads[0]
	buggy.SourceNamespace = buggy.Namespace

	builder := newBuilder(t, dyn)
	records := builder.Build(context.Background(), []model.WorkloadRecord{buggy})
	if len(records) != 1 {
		t.Fatalf("expected 1 chart record, got %d", len(records))
	}

	rec := records[0]
	if rec.ResolutionError == "" {
		t.Fatalf("expected resolution error when SourceNamespace is collapsed onto destination namespace; got clean record %+v", rec)
	}
	if !strings.Contains(rec.ResolutionError, "get argocd application") {
		t.Fatalf("expected error to mention argocd application lookup, got %q", rec.ResolutionError)
	}
	if rec.Status != model.VersionStatusUnknown {
		t.Fatalf("expected unknown status when lookup fails, got %q", rec.Status)
	}
}

// TestArgoCatalog_MetricsExposeDestinationNamespace ties the contract to
// what users actually see: the `namespace` label on `helm_chart_info`,
// `helm_chart_outdated`, and `helm_chart_version_lag` must be the workload
// destination namespace, never the Argo CD CR namespace. CI fails here if
// the metrics pipeline starts publishing the source namespace.
func TestArgoCatalog_MetricsExposeDestinationNamespace(t *testing.T) {
	srv := newFakeHelmRepo(t)
	app := newArgoApp("alloy", monitoringNamespace, srv.URL, "alloy", alloyCurrentVersion, false)
	dyn := newDynamicClientWithApps(app)

	disc := discovery.NewArgoCDApplicationDiscoverer(dyn)
	workloads, err := disc.Discover(context.Background())
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}

	builder := newBuilder(t, dyn)
	records := builder.Build(context.Background(), workloads)

	reg := prometheus.NewRegistry()
	m := metrics.NewChartMetrics(reg)
	m.Publish(workloads, records, version.NewEngine())

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	want := map[string]bool{
		"helm_chart_info":        false,
		"helm_chart_outdated":    false,
		"helm_chart_version_lag": false,
	}
	for _, fam := range families {
		if _, tracked := want[fam.GetName()]; !tracked {
			continue
		}
		if len(fam.GetMetric()) == 0 {
			t.Fatalf("metric family %q has no series", fam.GetName())
		}
		for _, metric := range fam.GetMetric() {
			ns := labelValue(metric.GetLabel(), "namespace")
			if ns != monitoringNamespace {
				t.Fatalf("%s namespace label = %q, want %q (destination namespace must be exposed, never source)", fam.GetName(), ns, monitoringNamespace)
			}
			if ns == argoCDNamespace {
				t.Fatalf("%s namespace label leaked the Argo CD CR namespace", fam.GetName())
			}
		}
		want[fam.GetName()] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("metric %q was not published", name)
		}
	}
}

func labelValue(labels []*dto.LabelPair, key string) string {
	for _, l := range labels {
		if l.GetName() == key {
			return l.GetValue()
		}
	}
	return ""
}
