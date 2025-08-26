package discovery

import (
	"testing"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

func TestInferDeploymentTypeFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected model.DeploymentType
	}{
		{
			name:     "terraform-managed",
			labels:   map[string]string{"app.kubernetes.io/managed-by": "Terraform"},
			expected: model.DeploymentTypeTerraform,
		},
		{
			name:     "helm-managed",
			labels:   map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			expected: model.DeploymentTypeHelm,
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: model.DeploymentTypeHelm,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferDeploymentTypeFromLabels(tc.labels)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestResolveReleaseName(t *testing.T) {
	got := resolveReleaseName(map[string]string{"name": "alloy"}, "fallback")
	if got != "alloy" {
		t.Fatalf("expected alloy, got %s", got)
	}

	got = resolveReleaseName(nil, "fallback")
	if got != "fallback" {
		t.Fatalf("expected fallback, got %s", got)
	}
}
