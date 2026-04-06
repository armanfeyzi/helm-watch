package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

// OCIResolver discovers chart versions stored as OCI artifacts.
//
// It speaks the Docker Registry V2 API (`/v2/<name>/tags/list`) with anonymous
// Bearer token negotiation so it works for public Helm charts on ghcr.io,
// registry-1.docker.io, quay.io, etc. Private registries will fail until token
// configuration is added.
type OCIResolver struct {
	client *http.Client
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]ociCacheEntry
}

type ociCacheEntry struct {
	tags      []string
	expiresAt time.Time
}

func NewOCIResolver(client *http.Client, ttl time.Duration) *OCIResolver {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &OCIResolver{
		client: client,
		ttl:    ttl,
		cache:  make(map[string]ociCacheEntry),
	}
}

// ResolveLatest returns the highest semver tag for chartName under repoURL.
//
// repoURL may be in the form `oci://host/path` or `host/path`. chartName is
// appended to the path so the artifact is `host/path/chartName`.
func (r *OCIResolver) ResolveLatest(ctx context.Context, repoURL, chartName string) (string, error) {
	host, repoPath, err := parseOCIRepo(repoURL)
	if err != nil {
		return "", err
	}

	chartName = strings.TrimSpace(chartName)
	if chartName == "" {
		return "", fmt.Errorf("chartName is required")
	}

	repository := strings.Trim(repoPath, "/") + "/" + chartName
	repository = strings.Trim(repository, "/")

	cacheKey := host + "/" + repository

	if tags, ok := r.cachedTags(cacheKey); ok {
		return pickLatestSemverTag(tags)
	}

	tags, err := r.fetchTags(ctx, host, repository)
	if err != nil {
		return "", err
	}

	r.storeTags(cacheKey, tags)
	return pickLatestSemverTag(tags)
}

func (r *OCIResolver) cachedTags(key string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[key]
	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(entry.expiresAt) {
		return nil, false
	}
	return entry.tags, true
}

func (r *OCIResolver) storeTags(key string, tags []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = ociCacheEntry{tags: tags, expiresAt: time.Now().UTC().Add(r.ttl)}
}

func (r *OCIResolver) fetchTags(ctx context.Context, host, repository string) ([]string, error) {
	endpoint := fmt.Sprintf("https://%s/v2/%s/tags/list", host, repository)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build tags request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch oci tags: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		token, tokenErr := r.negotiateAnonymousToken(ctx, resp, host, repository)
		_ = resp.Body.Close()
		if tokenErr != nil {
			return nil, tokenErr
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("rebuild tags request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = r.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch oci tags with token: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrChartNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oci tags status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oci tags body: %w", err)
	}

	var payload struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode oci tags: %w", err)
	}
	if len(payload.Tags) == 0 {
		return nil, ErrChartNotFound
	}

	return payload.Tags, nil
}

// negotiateAnonymousToken parses the WWW-Authenticate header from a 401 and
// requests an anonymous Bearer token. Works for ghcr.io and Docker Hub.
func (r *OCIResolver) negotiateAnonymousToken(ctx context.Context, resp *http.Response, host, repository string) (string, error) {
	challenge := resp.Header.Get("WWW-Authenticate")
	realm, service, scope := parseAuthChallenge(challenge)

	if realm == "" {
		// Sensible defaults so common registries still work without a challenge.
		switch host {
		case "ghcr.io":
			realm = "https://ghcr.io/token"
		case "registry-1.docker.io":
			realm = "https://auth.docker.io/token"
			service = "registry.docker.io"
		}
	}

	if realm == "" {
		return "", errors.New("oci auth: no realm in challenge")
	}
	if scope == "" {
		scope = "repository:" + repository + ":pull"
	}

	tokenURL := realm + "?scope=" + scope
	if service != "" {
		tokenURL += "&service=" + service
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}

	tokResp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch oci token: %w", err)
	}
	defer tokResp.Body.Close()

	if tokResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oci token status: %s", tokResp.Status)
	}

	body, err := io.ReadAll(tokResp.Body)
	if err != nil {
		return "", fmt.Errorf("read oci token body: %w", err)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode oci token: %w", err)
	}

	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", errors.New("oci auth: empty token in response")
}

// parseAuthChallenge extracts realm, service, and scope from a WWW-Authenticate
// header in the form `Bearer realm="...", service="...", scope="..."`.
func parseAuthChallenge(header string) (realm, service, scope string) {
	if header == "" {
		return "", "", ""
	}

	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer") {
		return "", "", ""
	}
	header = strings.TrimSpace(header[len("Bearer"):])

	parts := strings.Split(header, ",")
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		switch key {
		case "realm":
			realm = val
		case "service":
			service = val
		case "scope":
			scope = val
		}
	}
	return realm, service, scope
}

// parseOCIRepo splits an OCI repo URL into host and path. Accepts either
// `oci://host/path` or `host/path` (Argo CD often stores the latter).
func parseOCIRepo(repoURL string) (host, path string, err error) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", "", errors.New("repoURL is required")
	}

	repoURL = strings.TrimPrefix(repoURL, "oci://")
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.Trim(repoURL, "/")

	if repoURL == "" {
		return "", "", errors.New("repoURL is empty after stripping scheme")
	}

	idx := strings.Index(repoURL, "/")
	if idx == -1 {
		return repoURL, "", nil
	}

	host = repoURL[:idx]
	path = strings.Trim(repoURL[idx+1:], "/")
	return host, path, nil
}

// pickLatestSemverTag returns the highest semver-compatible tag, ignoring
// non-semver values like "latest" or branch names.
func pickLatestSemverTag(tags []string) (string, error) {
	var best *semver.Version
	var bestRaw string

	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		v, err := semver.NewVersion(strings.TrimPrefix(t, "v"))
		if err != nil {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
			bestRaw = t
		}
	}

	if best == nil {
		return "", ErrChartNotFound
	}
	return bestRaw, nil
}

// Ensure model.RepoCacheEntry stays referenced so future cache unification
// isn't accidentally dropped.
var _ = model.RepoCacheEntry{}
