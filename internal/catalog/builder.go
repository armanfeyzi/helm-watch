package catalog

import (
	"context"
	"errors"
	"fmt"
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
	versionEngine *version.Engine
	workers       int
}

func NewBuilder(dynamicClient dynamic.Interface, kubeClient kubernetes.Interface, repoResolver *resolver.RepositoryResolver, versionEngine *version.Engine, workers int) *Builder {
	if workers < 1 {
		workers = 1
	}
	return &Builder{
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		resolver:      repoResolver,
		versionEngine: versionEngine,
		workers:       workers,
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
		CurrentVersion: "unknown",
		LatestVersion:  "unknown",
		Status:         model.VersionStatusUnknown,
	}

	switch workload.SourceType {
	case model.SourceTypeArgoCDApplication:
		record = b.populateFromArgoCD(ctx, workload, record)
	case model.SourceTypeHelmReleaseSecret, model.SourceTypeHelmReleaseCM:
		record = b.populateFromHelmObject(workload, record)
	}

	if canResolve(record) {
		latest, err := b.resolver.ResolveLatest(ctx, record.RepoURL, record.ChartName)
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
	}
	if targetRevision != "" {
		record.CurrentVersion = targetRevision
	}
	return record
}

func (b *Builder) populateFromHelmObject(workload model.WorkloadRecord, record model.ChartRecord) model.ChartRecord {
	// Helm release objects do not consistently expose chart repo/version in labels.
	// We retain best-effort identification and mark unresolved fields as unknown.
	record.ChartName = workload.AppName
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
	chart, _ = source["chart"].(string)
	if strings.TrimSpace(chart) == "" {
		return "", "", "", false
	}
	repo, _ = source["repoURL"].(string)
	targetRevision, _ = source["targetRevision"].(string)
	return chart, repo, targetRevision, true
}

func canResolve(record model.ChartRecord) bool {
	if strings.TrimSpace(record.ChartName) == "" || strings.TrimSpace(record.RepoURL) == "" || record.RepoURL == "unknown" {
		return false
	}
	return true
}

func IsUnsupportedResolution(err error) bool {
	return errors.Is(err, resolver.ErrUnsupportedRepo) || errors.Is(err, resolver.ErrChartNotFound)
}
