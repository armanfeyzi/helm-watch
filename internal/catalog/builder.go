package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/resolver"
	"github.com/afeyzirealyticsio/helm-watch/internal/version"
)

var argoCDApplicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

type Builder struct {
	dynamicClient dynamic.Interface
	kubeClient    kubernetes.Interface
	resolver      *resolver.RepositoryResolver
	ociResolver   *resolver.OCIResolver
	versionEngine *version.Engine
	workers       int
	repoOverrides map[string]string
}

func NewBuilder(dynamicClient dynamic.Interface, kubeClient kubernetes.Interface, repoResolver *resolver.RepositoryResolver, ociResolver *resolver.OCIResolver, versionEngine *version.Engine, workers int, repoOverrides map[string]string) *Builder {
	if workers < 1 {
		workers = 1
	}
	if repoOverrides == nil {
		repoOverrides = map[string]string{}
	}
	return &Builder{
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		resolver:      repoResolver,
		ociResolver:   ociResolver,
		versionEngine: versionEngine,
		workers:       workers,
		repoOverrides: repoOverrides,
	}
}

func (b *Builder) Build(ctx context.Context, workloads []model.WorkloadRecord) []model.ChartRecord {
	type job struct {
		index    int
		workload model.WorkloadRecord
	}

	records := make([]model.ChartRecord, len(workloads))
	jobs := make(chan job)

	var wg sync.WaitGroup
	for i := 0; i < b.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				records[j.index] = b.buildSingle(ctx, j.workload)
			}
		}()
	}

	for i, workload := range workloads {
		jobs <- job{index: i, workload: workload}
	}
	close(jobs)
	wg.Wait()

	return records
}

func (b *Builder) buildSingle(ctx context.Context, workload model.WorkloadRecord) model.ChartRecord {
	record := model.ChartRecord{
		WorkloadID:     workload.ID,
		ChartName:      workload.AppName,
		RepoURL:        "unknown",
		SourceKind:     "unknown",
		CurrentVersion: "unknown",
		LatestVersion:  "unknown",
		Status:         model.VersionStatusUnknown,
	}

	switch workload.SourceType {
	case model.SourceTypeArgoCDApplication:
		record = b.populateFromArgoCD(ctx, workload, record)
	case model.SourceTypeHelmReleaseSecret, model.SourceTypeHelmReleaseCM:
		record = b.populateFromHelmObject(ctx, workload, record)
	}

	record = b.applyRepoOverride(record)

	if canResolve(record) {
		latest, err := b.resolveLatest(ctx, record)
		if err == nil {
			record.LatestVersion = latest
		} else if IsUnsupportedResolution(err) {
			// Unsupported/unknown is an expected state; keep status unknown without noisy error details.
			record.LatestVersion = "unknown"
		} else {
			record.ResolutionError = err.Error()
		}
	}

	result := b.versionEngine.Compare(record.CurrentVersion, record.LatestVersion)
	record.Status = result.Status
	return record
}

func (b *Builder) populateFromArgoCD(ctx context.Context, workload model.WorkloadRecord, record model.ChartRecord) model.ChartRecord {
	app, err := b.dynamicClient.Resource(argoCDApplicationGVR).Namespace(workload.Namespace).Get(ctx, workload.AppName, metav1.GetOptions{})
	if err != nil {
		record.ResolutionError = fmt.Sprintf("get argocd application: %v", err)
		return record
	}

	spec, ok := app.Object["spec"].(map[string]any)
	if !ok {
		record.ResolutionError = "argocd app missing spec"
		return record
	}

	chart, repo, targetRevision := extractArgoChartSource(spec)
	if chart != "" {
		record.ChartName = chart
	}
	if repo != "" {
		record.RepoURL = repo
		record.SourceKind = classifySourceKind(repo)
	}
	if targetRevision != "" {
		record.CurrentVersion = targetRevision
	}
	return record
}

func (b *Builder) populateFromHelmObject(ctx context.Context, workload model.WorkloadRecord, record model.ChartRecord) model.ChartRecord {
	objectName, ok := sourceObjectName(workload.ID)
	if !ok {
		record.ResolutionError = "invalid workload id format"
		return record
	}

	switch workload.SourceType {
	case model.SourceTypeHelmReleaseSecret:
		secret, err := b.kubeClient.CoreV1().Secrets(workload.Namespace).Get(ctx, objectName, metav1.GetOptions{})
		if err != nil {
			record.ResolutionError = fmt.Sprintf("get helm secret: %v", err)
			return record
		}
		record = applyHelmLabels(record, secret.Labels)
		if payload, ok := secret.Data["release"]; ok {
			record = applyHelmReleasePayload(record, payload)
		}

	case model.SourceTypeHelmReleaseCM:
		cm, err := b.kubeClient.CoreV1().ConfigMaps(workload.Namespace).Get(ctx, objectName, metav1.GetOptions{})
		if err != nil {
			record.ResolutionError = fmt.Sprintf("get helm configmap: %v", err)
			return record
		}
		record = applyHelmLabels(record, cm.Labels)
		if raw, ok := cm.Data["release"]; ok {
			record = applyHelmReleasePayload(record, []byte(raw))
		}
	}
	if record.RepoURL != "unknown" && record.SourceKind == "unknown" {
		record.SourceKind = classifySourceKind(record.RepoURL)
	}

	return record
}

func extractArgoChartSource(spec map[string]any) (chart, repo, targetRevision string) {
	if source, ok := spec["source"].(map[string]any); ok {
		if c, r, t, ok := parseArgoSource(source); ok {
			return c, r, t
		}
	}

	if sources, ok := spec["sources"].([]any); ok {
		for _, raw := range sources {
			sourceMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if c, r, t, ok := parseArgoSource(sourceMap); ok {
				return c, r, t
			}
		}
	}

	return "", "", ""
}

func parseArgoSource(source map[string]any) (chart, repo, targetRevision string, ok bool) {
	repo, _ = source["repoURL"].(string)
	targetRevision, _ = source["targetRevision"].(string)

	chart, _ = source["chart"].(string)
	if strings.TrimSpace(chart) != "" {
		return chart, repo, targetRevision, true
	}

	// Some Argo CD entries use a helm block without explicit chart.
	// In those cases we still return repo and targetRevision and let caller
	// fall back chart to app name.
	if _, hasHelm := source["helm"].(map[string]any); hasHelm && strings.TrimSpace(repo) != "" {
		return "", repo, targetRevision, true
	}

	return "", "", "", false
}

func canResolve(record model.ChartRecord) bool {
	if strings.TrimSpace(record.ChartName) == "" || strings.TrimSpace(record.RepoURL) == "" || record.RepoURL == "unknown" {
		return false
	}
	switch record.SourceKind {
	case "helm_repo", "oci_registry":
		return true
	default:
		return false
	}
}

func (b *Builder) resolveLatest(ctx context.Context, record model.ChartRecord) (string, error) {
	if record.SourceKind == "oci_registry" {
		if b.ociResolver == nil {
			return "", resolver.ErrUnsupportedRepo
		}
		return b.ociResolver.ResolveLatest(ctx, record.RepoURL, record.ChartName)
	}
	return b.resolver.ResolveLatest(ctx, record.RepoURL, record.ChartName)
}

func IsUnsupportedResolution(err error) bool {
	return errors.Is(err, resolver.ErrUnsupportedRepo) || errors.Is(err, resolver.ErrChartNotFound)
}

func sourceObjectName(workloadID string) (string, bool) {
	parts := strings.SplitN(workloadID, ":", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
		return "", false
	}
	return parts[2], true
}

func applyHelmLabels(record model.ChartRecord, labels map[string]string) model.ChartRecord {
	if labels == nil {
		return record
	}

	if chartLabel := labels["helm.sh/chart"]; chartLabel != "" {
		chartName, chartVersion := splitChartLabel(chartLabel)
		if chartName != "" {
			record.ChartName = chartName
		}
		if chartVersion != "" && record.CurrentVersion == "unknown" {
			record.CurrentVersion = chartVersion
		}
	}

	return record
}

func splitChartLabel(v string) (name, version string) {
	// helm.sh/chart usually looks like "<name>-<version>"
	// version starts with a digit in standard chart versions.
	for i := len(v) - 1; i >= 0; i-- {
		if v[i] != '-' {
			continue
		}
		if i+1 < len(v) && v[i+1] >= '0' && v[i+1] <= '9' {
			return v[:i], v[i+1:]
		}
	}
	return v, ""
}

type helmRelease struct {
	Chart struct {
		Metadata struct {
			Name    string   `json:"name"`
			Version string   `json:"version"`
			Sources []string `json:"sources"`
		} `json:"metadata"`
	} `json:"chart"`
}

func applyHelmReleasePayload(record model.ChartRecord, payload []byte) model.ChartRecord {
	decoded, err := decodeHelmRelease(payload)
	if err != nil {
		if record.ResolutionError == "" {
			record.ResolutionError = fmt.Sprintf("decode release payload: %v", err)
		}
		return record
	}

	var rel helmRelease
	if err := json.Unmarshal(decoded, &rel); err != nil {
		if record.ResolutionError == "" {
			record.ResolutionError = fmt.Sprintf("unmarshal release payload: %v", err)
		}
		return record
	}

	if rel.Chart.Metadata.Name != "" {
		record.ChartName = rel.Chart.Metadata.Name
	}
	if rel.Chart.Metadata.Version != "" {
		record.CurrentVersion = rel.Chart.Metadata.Version
	}
	if len(rel.Chart.Metadata.Sources) > 0 && record.RepoURL == "unknown" {
		repo := strings.TrimSpace(rel.Chart.Metadata.Sources[0])
		if repo != "" {
			record.RepoURL = repo
			record.SourceKind = classifySourceKind(repo)
		}
	}

	return record
}

func decodeHelmRelease(data []byte) ([]byte, error) {
	// Kubernetes API already decodes Secret.data once.
	// Helm payload itself is typically base64-encoded gzip-compressed JSON.
	inner, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}

	gr, err := gzip.NewReader(bytes.NewReader(inner))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	decoded, err := io.ReadAll(gr)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func classifySourceKind(repo string) string {
	r := strings.TrimSpace(repo)
	lr := strings.ToLower(r)

	if strings.HasPrefix(lr, "oci://") {
		return "oci_registry"
	}

	// Registry-like source without explicit oci:// prefix.
	if looksLikeRegistrySource(lr) {
		return "oci_registry"
	}

	u, err := url.Parse(r)
	if err != nil {
		return "unknown"
	}

	host := strings.ToLower(u.Host)
	path := strings.ToLower(u.Path)

	if host == "" {
		return "unknown"
	}

	if strings.Contains(host, "github.com") || strings.HasSuffix(path, ".git") {
		return "git"
	}

	if strings.Contains(host, "github.io") || strings.Contains(path, "helm") || strings.Contains(path, "charts") {
		return "helm_repo"
	}

	if strings.HasPrefix(lr, "http://") || strings.HasPrefix(lr, "https://") {
		return "helm_repo"
	}

	return "unknown"
}

func looksLikeRegistrySource(repo string) bool {
	switch {
	case strings.Contains(repo, "ghcr.io/"),
		strings.Contains(repo, "registry-1.docker.io/"),
		strings.Contains(repo, "docker.io/"),
		strings.Contains(repo, "quay.io/"),
		strings.Contains(repo, "public.ecr.aws/"):
		return true
	default:
		return false
	}
}

func (b *Builder) applyRepoOverride(record model.ChartRecord) model.ChartRecord {
	if record.RepoURL != "unknown" && strings.TrimSpace(record.RepoURL) != "" {
		return record
	}
	if len(b.repoOverrides) == 0 {
		return record
	}

	chartKey := strings.ToLower(strings.TrimSpace(record.ChartName))
	if chartKey == "" {
		return record
	}
	if repo, ok := b.repoOverrides[chartKey]; ok && strings.TrimSpace(repo) != "" {
		record.RepoURL = repo
		record.SourceKind = classifySourceKind(repo)
	}
	return record
}
