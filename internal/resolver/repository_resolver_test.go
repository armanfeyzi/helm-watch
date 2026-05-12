package resolver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/afeyzirealyticsio/helm-watch/internal/registryauth"
)

const sampleIndexYAML = `
apiVersion: v1
entries:
  alloy:
    - version: 1.8.2
    - version: 1.6.0
  grafana:
    - version: 7.0.0
`

func TestResolveLatestFromIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.yaml" {
			t.Fatalf("expected /index.yaml, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleIndexYAML))
	}))
	defer srv.Close()

	resolver := NewRepositoryResolver(nil, 1*time.Minute, nil)
	got, err := resolver.ResolveLatest(context.Background(), srv.URL, "alloy")
	if err != nil {
		t.Fatalf("resolve latest failed: %v", err)
	}
	if got != "1.8.2" {
		t.Fatalf("expected 1.8.2, got %s", got)
	}
}

func TestResolveLatestUsesCacheWhenFresh(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_, _ = w.Write([]byte(sampleIndexYAML))
	}))
	defer srv.Close()

	resolver := NewRepositoryResolver(nil, 10*time.Minute, nil)
	ctx := context.Background()

	_, err := resolver.ResolveLatest(ctx, srv.URL, "alloy")
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	_, err = resolver.ResolveLatest(ctx, srv.URL, "alloy")
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("expected exactly 1 upstream request due to cache, got %d", requestCount)
	}
}

func TestResolveLatestStaleOnError(t *testing.T) {
	failMode := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failMode {
			http.Error(w, "upstream error", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(sampleIndexYAML))
	}))
	defer srv.Close()

	resolver := NewRepositoryResolver(nil, 1*time.Nanosecond, nil)
	ctx := context.Background()

	_, err := resolver.ResolveLatest(ctx, srv.URL, "alloy")
	if err != nil {
		t.Fatalf("initial resolve failed: %v", err)
	}

	time.Sleep(2 * time.Millisecond) // ensure cache entry is stale
	failMode = true

	got, err := resolver.ResolveLatest(ctx, srv.URL, "alloy")
	if err != nil {
		t.Fatalf("expected stale fallback, got error: %v", err)
	}
	if got != "1.8.2" {
		t.Fatalf("expected stale fallback version 1.8.2, got %s", got)
	}
}

func TestResolveLatestUnsupportedOCI(t *testing.T) {
	resolver := NewRepositoryResolver(nil, 1*time.Minute, nil)
	_, err := resolver.ResolveLatest(context.Background(), "oci://ghcr.io/org/charts", "alloy")
	if !errors.Is(err, ErrUnsupportedRepo) {
		t.Fatalf("expected ErrUnsupportedRepo, got %v", err)
	}
}

func TestResolveLatestHelmIndexWithBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "robot" || p != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/index.yaml" {
			t.Fatalf("expected /index.yaml, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleIndexYAML))
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	auth := map[string]registryauth.Credential{
		registryauth.NormalizeHost(parsed.Host): {Username: "robot", Password: "s3cret"},
	}
	resolver := NewRepositoryResolver(srv.Client(), time.Minute, auth)

	got, err := resolver.ResolveLatest(context.Background(), srv.URL, "alloy")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "1.8.2" {
		t.Fatalf("got %s", got)
	}
}
