package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

type blockingDiscoverer struct {
	release chan struct{}
}

func (b *blockingDiscoverer) Discover(ctx context.Context) ([]model.WorkloadRecord, error) {
	select {
	case <-b.release:
		return []model.WorkloadRecord{{ID: "a"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestWaitFirstReconcileBlocksUntilDiscoveryFinishes(t *testing.T) {
	release := make(chan struct{})
	m := NewManager(&blockingDiscoverer{release: release}, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		m.Run(ctx)
	}()

	waitReturned := make(chan struct{})
	go func() {
		m.WaitFirstReconcile(ctx)
		close(waitReturned)
	}()

	select {
	case <-waitReturned:
		t.Fatal("WaitFirstReconcile returned before first discovery finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitFirstReconcile did not return after discovery")
	}

	if n := len(m.Snapshot()); n != 1 {
		t.Fatalf("snapshot: want 1 workload, got %d", n)
	}
}
