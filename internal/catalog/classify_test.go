package catalog

import "testing"

func TestClassifySourceKind(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ssh://git@bitbucket.org/realyticsio/gitops.git", "git"},
		{"https://github.com/armanfeyzi/trivy-dashboard.git", "git"},
		{"https://prometheus-community.github.io/helm-charts", "helm_repo"},
		{"https://helm.releases.hashicorp.com", "helm_repo"},
		{"ghcr.io/grafana/helm-charts", "oci_registry"},
		{"registry-1.docker.io/bitnamicharts", "oci_registry"},
		{"oci://ghcr.io/grafana/helm-charts", "oci_registry"},
	}

	for _, tc := range tests {
		got := classifySourceKind(tc.in)
		if got != tc.want {
			t.Fatalf("classifySourceKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
