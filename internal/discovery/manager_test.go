package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

// TestDeduplicateByApp verifies that when the same app is found by both the
// ArgoCD and Helm release discoverers the higher-priority source (ArgoCD) wins
// and only one WorkloadRecord is emitted per (namespace, app) pair.
func TestDeduplicateByApp(t *testing.T) {
	argoRecord := model.WorkloadRecord{
		ID:         "argocd_application:argocd:external-secrets",
		AppName:    "external-secrets",
		Namespace:  "external-secrets",
		SourceType: model.SourceTypeArgoCDApplication,
	}
	helmRecord := model.WorkloadRecord{
		ID:         "helm_release_secret:external-secrets:sh.helm.release.v1.external-secrets.v1",
		AppName:    "external-secrets",
		Namespace:  "external-secrets",
		SourceType: model.SourceTypeHelmReleaseSecret,
	}

	got := deduplicateByApp([]model.WorkloadRecord{argoRecord, helmRecord})
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	if got[0].SourceType != model.SourceTypeArgoCDApplication {
		t.Fatalf("want ArgoCD source to win, got %s", got[0].SourceType)
	}
}

// TestCompositeDeduplicatesCrossSourceApps is an integration-style test that
// wires two stub discoverers through Composite and confirms the (namespace,
// app) deduplication fires end-to-end.
func TestCompositeDeduplicatesCrossSourceApps(t *testing.T) {
	argoRecord := model.WorkloadRecord{
		ID:         "argocd_application:argocd:helm-watch",
		AppName:    "helm-watch",
		Namespace:  "helm-watch",
		SourceType: model.SourceTypeArgoCDApplication,
	}
	helmRecord := model.WorkloadRecord{
		ID:         "helm_release_secret:helm-watch:sh.helm.release.v1.helm-watch.v3",
		AppName:    "helm-watch",
		Namespace:  "helm-watch",
		SourceType: model.SourceTypeHelmReleaseSecret,
	}

	composite := NewComposite(
		&staticDiscoverer{records: []model.WorkloadRecord{argoRecord}},
		&staticDiscoverer{records: []model.WorkloadRecord{helmRecord}},
	)

	records, err := composite.Discover(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record after cross-source dedup, got %d", len(records))
	}
	if records[0].SourceType != model.SourceTypeArgoCDApplication {
		t.Fatalf("want ArgoCD source to win, got %s", records[0].SourceType)
	}
}

type staticDiscoverer struct {
	records []model.WorkloadRecord
}

func (s *staticDiscoverer) Discover(_ context.Context) ([]model.WorkloadRecord, error) {
	return s.records, nil
}

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
