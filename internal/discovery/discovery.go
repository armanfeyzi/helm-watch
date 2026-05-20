package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

type Discoverer interface {
	Discover(ctx context.Context) ([]model.WorkloadRecord, error)
}

type Composite struct {
	sources []Discoverer
}

func NewComposite(sources ...Discoverer) *Composite {
	return &Composite{sources: sources}
}

func (c *Composite) Discover(ctx context.Context) ([]model.WorkloadRecord, error) {
	var out []model.WorkloadRecord
	failures := 0
	for _, source := range c.sources {
		records, err := source.Discover(ctx)
		if err != nil {
			failures++
			slog.Warn("discovery source failed", "error", err)
			continue
		}
		out = append(out, records...)
	}

	if failures == len(c.sources) {
		return nil, fmt.Errorf("all discovery sources failed")
	}

	slices.SortFunc(out, func(a, b model.WorkloadRecord) int {
		return compareID(a.ID, b.ID)
	})

	return deduplicateByApp(deduplicate(out)), nil
}

func compareID(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func deduplicate(in []model.WorkloadRecord) []model.WorkloadRecord {
	if len(in) == 0 {
		return in
	}

	out := make([]model.WorkloadRecord, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, rec := range in {
		if _, ok := seen[rec.ID]; ok {
			continue
		}
		seen[rec.ID] = struct{}{}
		out = append(out, rec)
	}
	return out
}

// sourceTypePriority returns a lower number for higher-priority source types.
// ArgoCD is preferred because its Application CR carries authoritative chart
// source information (repoURL, chart name, targetRevision), whereas a Helm
// release secret/configmap may only expose what is baked into the chart
// metadata and can disagree on the repo URL.
func sourceTypePriority(s model.SourceType) int {
	switch s {
	case model.SourceTypeArgoCDApplication:
		return 0
	case model.SourceTypeHelmReleaseSecret:
		return 1
	case model.SourceTypeHelmReleaseCM:
		return 2
	default:
		return 3
	}
}

// deduplicateByApp removes duplicate workloads that represent the same logical
// application (same Namespace + AppName) discovered by multiple sources. When
// a collision occurs the record with the highest source-type priority wins so
// that each (namespace, app) pair maps to exactly one ChartRecord and the
// narrow-label gauges (helm_chart_outdated, helm_chart_unknown) stay in sync
// with the wide-label gauge (helm_chart_info).
func deduplicateByApp(in []model.WorkloadRecord) []model.WorkloadRecord {
	if len(in) == 0 {
		return in
	}

	type appKey struct{ namespace, app string }
	best := make(map[appKey]model.WorkloadRecord, len(in))
	for _, rec := range in {
		k := appKey{rec.Namespace, rec.AppName}
		if existing, ok := best[k]; !ok || sourceTypePriority(rec.SourceType) < sourceTypePriority(existing.SourceType) {
			best[k] = rec
		}
	}

	out := make([]model.WorkloadRecord, 0, len(best))
	for _, rec := range best {
		out = append(out, rec)
	}
	slices.SortFunc(out, func(a, b model.WorkloadRecord) int {
		return compareID(a.ID, b.ID)
	})
	return out
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func workloadID(source model.SourceType, namespace, name string) string {
	return fmt.Sprintf("%s:%s:%s", source, namespace, name)
}
