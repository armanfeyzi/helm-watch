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

	// firstReconcileDone is closed after the first reconcileOnce attempt finishes
	// (success or error) so the metrics pipeline can avoid publishing an empty
	// snapshot before discovery has populated at least once.
	firstReconcileDone chan struct{}
	firstOnce          sync.Once
}

func NewManager(discoverer Discoverer, interval time.Duration) *Manager {
	return &Manager{
		discoverer:         discoverer,
		interval:           interval,
		firstReconcileDone: make(chan struct{}),
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
	defer m.signalFirstReconcileDone()

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

func (m *Manager) signalFirstReconcileDone() {
	m.firstOnce.Do(func() { close(m.firstReconcileDone) })
}

// WaitFirstReconcile blocks until the first discovery reconcile attempt has
// finished, or ctx is cancelled. Used so metrics publication does not run with
// an empty workload snapshot on startup.
func (m *Manager) WaitFirstReconcile(ctx context.Context) {
	select {
	case <-m.firstReconcileDone:
	case <-ctx.Done():
	}
}
