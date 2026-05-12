package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/registryauth"
)

// OCIResolver discovers chart versions stored as OCI artifacts.
//
// It speaks the Docker Registry V2 API (`/v2/<name>/tags/list`) with anonymous
// Bearer token negotiation so it works for public Helm charts on ghcr.io,
// registry-1.docker.io, quay.io, etc. For private registries, configure
// host-scoped HTTP Basic credentials (token endpoint and optional direct /v2/
// access).
type OCIResolver struct {
	client   *http.Client
	ttl      time.Duration
	hostAuth map[string]registryauth.Credential

	mu    sync.RWMutex
	cache map[string]ociCacheEntry
}

type ociCacheEntry struct {
	tags      []string
	expiresAt time.Time
}

func NewOCIResolver(client *http.Client, ttl time.Duration, hostAuth map[string]registryauth.Credential) *OCIResolver {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &OCIResolver{
		client:   client,
		ttl:      ttl,
		hostAuth: hostAuth,
		cache:    make(map[string]ociCacheEntry),
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

// maxOCIPages caps pagination to keep memory and latency bounded. Even
// long-lived charts rarely exceed a few thousand tags, and we only need the
// highest semver across the full set.
const maxOCIPages = 50

func (r *OCIResolver) fetchTags(ctx context.Context, host, repository string) ([]string, error) {
	startURL := fmt.Sprintf("https://%s/v2/%s/tags/list", host, repository)

	var token string
	var useBasic bool
	all := make([]string, 0, 256)
	nextURL := startURL

	for page := 0; page < maxOCIPages && nextURL != ""; page++ {
		var body []byte
		var linkHeader string
		stopPaging := false

	pages:
		for attempt := 0; attempt < 8; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
			if err != nil {
				return nil, fmt.Errorf("build tags request: %w", err)
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			} else if useBasic {
				if c, ok := registryauth.Lookup(r.hostAuth, host); ok && c.Username != "" {
					req.SetBasicAuth(c.Username, c.Password)
				}
			}

			resp, err := r.client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetch oci tags: %w", err)
			}

			if resp.StatusCode == http.StatusUnauthorized {
				if token == "" && !useBasic {
					www := resp.Header.Get("WWW-Authenticate")
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()

					bearerOK := false
					var bearErr error
					if www != "" {
						var t string
						t, bearErr = r.negotiateBearerToken(ctx, www, host, repository)
						if bearErr == nil && t != "" {
							token = t
							bearerOK = true
						}
					}
					if bearerOK {
						continue
					}
					if c, ok := registryauth.Lookup(r.hostAuth, host); ok && c.Username != "" {
						useBasic = true
						if bearErr != nil {
							// Prefer basic path over a failed anonymous token exchange.
							_ = bearErr
						}
						continue
					}
					if bearErr != nil {
						return nil, bearErr
					}
					return nil, fmt.Errorf("oci tags status: %s", resp.Status)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				return nil, fmt.Errorf("oci tags status: %s", resp.Status)
			}

			if resp.StatusCode == http.StatusNotFound {
				_ = resp.Body.Close()
				if len(all) > 0 {
					stopPaging = true
					break pages
				}
				return nil, ErrChartNotFound
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return nil, fmt.Errorf("oci tags status: %s", resp.Status)
			}

			body, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("read oci tags body: %w", err)
			}
			linkHeader = resp.Header.Get("Link")
			break pages
		}

		if stopPaging {
			break
		}
		if body == nil {
			return nil, fmt.Errorf("oci tags: exhausted auth retries for %s", nextURL)
		}

		var payload struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode oci tags: %w", err)
		}
		all = append(all, payload.Tags...)

		nextURL = resolveNextPageURL(startURL, linkHeader)
	}

	if len(all) == 0 {
		return nil, ErrChartNotFound
	}
	return all, nil
}

// resolveNextPageURL extracts the `rel="next"` target from a Link header and
// resolves it against the original request URL. Registries return either an
// absolute URL or a path (`/v2/.../tags/list?n=100&last=...`).
func resolveNextPageURL(currentURL, linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	for _, part := range strings.Split(linkHeader, ",") {
		segs := strings.Split(strings.TrimSpace(part), ";")
		if len(segs) < 2 {
			continue
		}
		raw := strings.TrimSpace(segs[0])
		raw = strings.TrimPrefix(raw, "<")
		raw = strings.TrimSuffix(raw, ">")

		isNext := false
		for _, attr := range segs[1:] {
			if strings.Contains(strings.ToLower(attr), `rel="next"`) ||
				strings.Contains(strings.ToLower(attr), "rel=next") {
				isNext = true
				break
			}
		}
		if !isNext || raw == "" {
			continue
		}

		base, err := neturl.Parse(currentURL)
		if err != nil {
			return raw
		}
		ref, err := neturl.Parse(raw)
		if err != nil {
			return raw
		}
		return base.ResolveReference(ref).String()
	}
	return ""
}

// negotiateBearerToken parses the WWW-Authenticate challenge, requests a Bearer
// token from the realm, and optionally sends HTTP Basic credentials when
// hostAuth is configured for this registry (private OCI).
func (r *OCIResolver) negotiateBearerToken(ctx context.Context, wwwAuthenticate, host, repository string) (string, error) {
	challenge := wwwAuthenticate
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
	if c, ok := registryauth.Lookup(r.hostAuth, host); ok && c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
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
