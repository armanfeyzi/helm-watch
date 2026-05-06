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

	return deduplicate(out), nil
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

func nowUTC() time.Time {
	return time.Now().UTC()
}

func workloadID(source model.SourceType, namespace, name string) string {
	return fmt.Sprintf("%s:%s:%s", source, namespace, name)
}
