// Package registryauth parses per-registry HTTP Basic credentials for private
// Helm index and OCI chart resolution.
package registryauth

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// Credential is HTTP Basic auth for a single registry host (Helm HTTP or OCI).
type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ParseCredentialsJSON parses a JSON object keyed by registry hostname, for example:
//
//	{"ghcr.io":{"username":"robot","password":"..."},"charts.internal":{"username":"u","password":"p"}}
//
// Keys are normalized with NormalizeHost (lowercase, host without port).
func ParseCredentialsJSON(b []byte) (map[string]Credential, error) {
	b = []byte(strings.TrimSpace(string(b)))
	if len(b) == 0 {
		return nil, nil
	}

	var raw map[string]struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("registry credentials json: %w", err)
	}

	out := make(map[string]Credential, len(raw))
	for k, v := range raw {
		host := NormalizeHost(k)
		if host == "" {
			continue
		}
		out[host] = Credential{Username: v.Username, Password: v.Password}
	}
	return out, nil
}

// NormalizeHost returns a lowercase hostname without a port for matching keys.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// Lookup returns credentials for a request host (may include port).
func Lookup(m map[string]Credential, host string) (Credential, bool) {
	if len(m) == 0 {
		return Credential{}, false
	}
	c, ok := m[NormalizeHost(host)]
	return c, ok
}
