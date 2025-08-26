package discovery

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

type Manager struct {
	discoverer Discoverer
	interval   time.Duration

	mu      sync.RWMutex
	records []model.WorkloadRecord
}

func NewManager(discoverer Discoverer, interval time.Duration) *Manager {
	return &Manager{
		discoverer: discoverer,
		interval:   interval,
	}
}

func (m *Manager) Run(ctx context.Context) {
	m.reconcileOnce(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reconcileOnce(ctx)
		}
	}
}

func (m *Manager) Snapshot() []model.WorkloadRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.WorkloadRecord, len(m.records))
	copy(out, m.records)
	return out
}

func (m *Manager) reconcileOnce(ctx context.Context) {
	start := time.Now()
	records, err := m.discoverer.Discover(ctx)
	if err != nil {
		slog.Error("discovery reconcile failed", "error", err)
		return
	}

	m.mu.Lock()
	m.records = records
	m.mu.Unlock()

	slog.Info(
		"discovery reconcile completed",
		"workload_count", len(records),
		"duration_ms", time.Since(start).Milliseconds(),
	)
}
