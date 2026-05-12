package registryauth

import (
	"testing"
)

func TestParseCredentialsJSON(t *testing.T) {
	raw := []byte(`{"GHCR.io":{"username":"u","password":"p"},"charts.internal:443":{"username":"a","password":"b"}}`)
	m, err := ParseCredentialsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("want 2 hosts, got %d", len(m))
	}
	if c, ok := m["ghcr.io"]; !ok || c.Username != "u" || c.Password != "p" {
		t.Fatalf("ghcr.io creds: %+v ok=%v", c, ok)
	}
	if c, ok := m["charts.internal"]; !ok || c.Username != "a" {
		t.Fatalf("charts.internal: %+v", c)
	}
}

func TestLookup(t *testing.T) {
	m := map[string]Credential{"registry.example.com": {Username: "x", Password: "y"}}
	c, ok := Lookup(m, "Registry.EXAMPLE.com:443")
	if !ok || c.Username != "x" {
		t.Fatalf("Lookup = %+v ok=%v", c, ok)
	}
}
