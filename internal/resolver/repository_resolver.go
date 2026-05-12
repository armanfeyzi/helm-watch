package resolver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
	"github.com/afeyzirealyticsio/helm-watch/internal/registryauth"
)

var (
	ErrUnsupportedRepo = errors.New("unsupported repository source")
	ErrChartNotFound   = errors.New("chart not found in repository index")
)

type RepositoryResolver struct {
	client   *http.Client
	ttl      time.Duration
	hostAuth map[string]registryauth.Credential

	mu    sync.RWMutex
	cache map[string]model.RepoCacheEntry
}

// NewRepositoryResolver resolves Helm HTTP repository indexes. hostAuth maps
// canonical registry hostnames (see registryauth) to HTTP Basic credentials for
// private index.yaml endpoints; nil or empty means anonymous only.
func NewRepositoryResolver(client *http.Client, ttl time.Duration, hostAuth map[string]registryauth.Credential) *RepositoryResolver {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &RepositoryResolver{
		client:   client,
		ttl:      ttl,
		hostAuth: hostAuth,
		cache:    make(map[string]model.RepoCacheEntry),
	}
}

func (r *RepositoryResolver) ResolveLatest(ctx context.Context, repoURL, chartName string) (string, error) {
	if repoURL == "" || chartName == "" {
		return "", fmt.Errorf("repoURL and chartName are required")
	}

	if strings.HasPrefix(repoURL, "oci://") {
		return "", ErrUnsupportedRepo
	}

	entry, err := r.getOrRefresh(ctx, repoURL)
	if err != nil {
		return "", err
	}

	versions := entry.Charts[chartName]
	if len(versions) == 0 {
		return "", ErrChartNotFound
	}

	// Helm index.yaml typically lists entries in descending (latest-first) order.
	return versions[0], nil
}

func (r *RepositoryResolver) RefreshRepoIndex(ctx context.Context, repoURL string) (model.RepoCacheEntry, error) {
	if strings.HasPrefix(repoURL, "oci://") {
		return model.RepoCacheEntry{}, ErrUnsupportedRepo
	}

	indexURL, err := indexURLForRepo(repoURL)
	if err != nil {
		return model.RepoCacheEntry{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return model.RepoCacheEntry{}, fmt.Errorf("build request: %w", err)
	}
	if u, err := url.Parse(indexURL); err == nil {
		if c, ok := registryauth.Lookup(r.hostAuth, u.Hostname()); ok && c.Username != "" {
			req.SetBasicAuth(c.Username, c.Password)
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return model.RepoCacheEntry{}, fmt.Errorf("fetch index.yaml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.RepoCacheEntry{}, fmt.Errorf("fetch index.yaml status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.RepoCacheEntry{}, fmt.Errorf("read index.yaml: %w", err)
	}

	charts, err := parseRepoIndex(body)
	if err != nil {
		return model.RepoCacheEntry{}, fmt.Errorf("parse index.yaml: %w", err)
	}

	now := time.Now().UTC()
	entry := model.RepoCacheEntry{
		RepoURL:   repoURL,
		FetchedAt: now,
		ExpiresAt: now.Add(r.ttl),
		Charts:    charts,
	}

	r.mu.Lock()
	r.cache[repoURL] = entry
	r.mu.Unlock()

	return entry, nil
}

func (r *RepositoryResolver) getOrRefresh(ctx context.Context, repoURL string) (model.RepoCacheEntry, error) {
	if entry, ok := r.getFresh(repoURL); ok {
		return entry, nil
	}

	entry, err := r.RefreshRepoIndex(ctx, repoURL)
	if err == nil {
		return entry, nil
	}

	// stale-on-error fallback
	if stale, ok := r.getStale(repoURL); ok {
		stale.LastError = err.Error()
		r.mu.Lock()
		r.cache[repoURL] = stale
		r.mu.Unlock()
		return stale, nil
	}

	return model.RepoCacheEntry{}, err
}

func (r *RepositoryResolver) getFresh(repoURL string) (model.RepoCacheEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[repoURL]
	if !ok {
		return model.RepoCacheEntry{}, false
	}
	if time.Now().UTC().After(entry.ExpiresAt) {
		return model.RepoCacheEntry{}, false
	}
	return entry, true
}

func (r *RepositoryResolver) getStale(repoURL string) (model.RepoCacheEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.cache[repoURL]
	return entry, ok
}

func indexURLForRepo(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repo url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("repo url must be absolute: %s", repoURL)
	}

	if strings.HasSuffix(u.Path, "/index.yaml") {
		return u.String(), nil
	}

	if strings.HasSuffix(u.Path, "/") {
		u.Path += "index.yaml"
	} else {
		u.Path += "/index.yaml"
	}

	return u.String(), nil
}

type repoIndex struct {
	Entries map[string][]repoIndexEntry `yaml:"entries"`
}

type repoIndexEntry struct {
	Version string `yaml:"version"`
}

func parseRepoIndex(data []byte) (map[string][]string, error) {
	var idx repoIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	out := make(map[string][]string, len(idx.Entries))
	for chartName, entries := range idx.Entries {
		versions := make([]string, 0, len(entries))
		for _, entry := range entries {
			if strings.TrimSpace(entry.Version) == "" {
				continue
			}
			versions = append(versions, entry.Version)
		}
		if len(versions) > 0 {
			out[chartName] = versions
		}
	}
	return out, nil
}
