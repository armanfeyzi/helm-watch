package discovery

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestApplicationUsesHelm(t *testing.T) {
	t.Run("single source with chart", func(t *testing.T) {
		app := unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"source": map[string]any{
						"repoURL": "https://example.com",
						"chart":   "my-chart",
					},
				},
			},
		}
		if !applicationUsesHelm(app) {
			t.Fatal("expected helm source to be detected")
		}
	})

	t.Run("multi source with helm entry", func(t *testing.T) {
		app := unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"sources": []any{
						map[string]any{"repoURL": "https://git.example.com/repo"},
						map[string]any{"repoURL": "https://helm.example.com", "chart": "agent"},
					},
				},
			},
		}
		if !applicationUsesHelm(app) {
			t.Fatal("expected helm source in multi-source application to be detected")
		}
	})

	t.Run("non helm app", func(t *testing.T) {
		app := unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"source": map[string]any{
						"repoURL": "https://git.example.com/repo",
						"path":    "apps/my-app",
					},
				},
			},
		}
		if applicationUsesHelm(app) {
			t.Fatal("did not expect non-helm source to be detected as helm")
		}
	})
}
