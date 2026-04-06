package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseOCIRepo(t *testing.T) {
	tests := []struct {
		in       string
		wantHost string
		wantPath string
		wantErr  bool
	}{
		{"oci://ghcr.io/grafana/helm-charts", "ghcr.io", "grafana/helm-charts", false},
		{"ghcr.io/grafana/helm-charts", "ghcr.io", "grafana/helm-charts", false},
		{"registry-1.docker.io/bitnamicharts", "registry-1.docker.io", "bitnamicharts", false},
		{"https://ghcr.io/grafana/helm-charts/", "ghcr.io", "grafana/helm-charts", false},
		{"", "", "", true},
	}

	for _, tc := range tests {
		host, path, err := parseOCIRepo(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseOCIRepo(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseOCIRepo(%q) returned error: %v", tc.in, err)
		}
		if host != tc.wantHost || path != tc.wantPath {
			t.Fatalf("parseOCIRepo(%q) = (%q,%q), want (%q,%q)", tc.in, host, path, tc.wantHost, tc.wantPath)
		}
	}
}

func TestParseAuthChallenge(t *testing.T) {
	header := `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:grafana/helm-charts/tempo-distributed:pull"`
	realm, service, scope := parseAuthChallenge(header)
	if realm != "https://ghcr.io/token" || service != "ghcr.io" || !strings.HasPrefix(scope, "repository:") {
		t.Fatalf("unexpected challenge parse result: realm=%q service=%q scope=%q", realm, service, scope)
	}
}

func TestPickLatestSemverTag(t *testing.T) {
	tags := []string{"latest", "1.61.3", "v1.62.0", "main", "1.59.9", "1.62.0-rc.1"}
	got, err := pickLatestSemverTag(tags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.62.0" {
		t.Fatalf("pickLatestSemverTag = %q, want v1.62.0", got)
	}
}

func TestPickLatestSemverTagEmpty(t *testing.T) {
	if _, err := pickLatestSemverTag([]string{"main", "latest"}); err == nil {
		t.Fatalf("expected error when no semver tags")
	}
}

func TestResolveNextPageURL(t *testing.T) {
	cur := "https://ghcr.io/v2/grafana/helm-charts/tempo-distributed/tags/list"

	tests := []struct {
		name string
		link string
		want string
	}{
		{
			name: "absolute next",
			link: `<https://ghcr.io/v2/grafana/helm-charts/tempo-distributed/tags/list?n=100&last=1.46.1>; rel="next"`,
			want: "https://ghcr.io/v2/grafana/helm-charts/tempo-distributed/tags/list?n=100&last=1.46.1",
		},
		{
			name: "relative next",
			link: `</v2/grafana/helm-charts/tempo-distributed/tags/list?n=100&last=1.46.1>; rel="next"`,
			want: "https://ghcr.io/v2/grafana/helm-charts/tempo-distributed/tags/list?n=100&last=1.46.1",
		},
		{
			name: "no next rel",
			link: `</v2/foo>; rel="prev"`,
			want: "",
		},
		{
			name: "empty",
			link: "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveNextPageURL(cur, tc.link)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOCIResolverFollowsPagination(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/grafana/helm-charts/tempo-distributed/tags/list" {
			http.NotFound(w, r)
			return
		}
		hits++
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Query().Get("last") {
		case "":
			// Page 1: lexically smaller tags.
			w.Header().Set("Link", `</v2/grafana/helm-charts/tempo-distributed/tags/list?n=2&last=1.46.1>; rel="next"`)
			_, _ = w.Write([]byte(`{"tags":["1.0.0","1.46.1"]}`))
		case "1.46.1":
			// Page 2: the actually-newest tags. No further Link header.
			_, _ = w.Write([]byte(`{"tags":["1.50.0","1.62.0"]}`))
		default:
			http.Error(w, "unexpected last", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	r := NewOCIResolver(srv.Client(), time.Minute)
	r.client = &http.Client{Timeout: 5 * time.Second, Transport: rewriteTransport{base: srv.Client().Transport, host: host}}

	got, err := r.ResolveLatest(context.Background(), host+"/grafana/helm-charts", "tempo-distributed")
	if err != nil {
		t.Fatalf("ResolveLatest returned error: %v", err)
	}
	if got != "1.62.0" {
		t.Fatalf("ResolveLatest = %q, want 1.62.0", got)
	}
	if hits != 2 {
		t.Fatalf("expected 2 page fetches, got %d", hits)
	}
}

func TestOCIResolverResolveLatestPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/charts/redis/tags/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"charts/redis","tags":["20.6.3","20.7.0","21.0.0","main"]}`))
	}))
	defer srv.Close()

	r := NewOCIResolver(srv.Client(), time.Minute)
	host := strings.TrimPrefix(srv.URL, "http://")

	// parseOCIRepo strips http(s) and oci, but the resolver always issues HTTPS.
	// Inject the http test server by using a custom transport that rewrites scheme.
	r.client = &http.Client{Timeout: 5 * time.Second, Transport: rewriteTransport{base: srv.Client().Transport, host: host}}

	got, err := r.ResolveLatest(context.Background(), host+"/charts", "redis")
	if err != nil {
		t.Fatalf("ResolveLatest returned error: %v", err)
	}
	if got != "21.0.0" {
		t.Fatalf("ResolveLatest = %q, want 21.0.0", got)
	}
}

// rewriteTransport rewrites https:// requests to http:// for the test server.
type rewriteTransport struct {
	base http.RoundTripper
	host string
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" && req.URL.Host == rt.host {
		req.URL.Scheme = "http"
	}
	if rt.base == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return rt.base.RoundTrip(req)
}
