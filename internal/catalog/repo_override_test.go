package catalog

import (
	"testing"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

func TestApplyRepoOverride(t *testing.T) {
	overrides := map[string]string{
		"vault":                "https://helm.releases.hashicorp.com",
		"argocd-image-updater": "https://argoproj.github.io/argo-helm",
	}

	tests := []struct {
		name           string
		input          model.ChartRecord
		wantRepo       string
		wantSourceKind string
	}{
		{
			name: "applies when repo is unknown",
			input: model.ChartRecord{
				ChartName:  "argocd-image-updater",
				RepoURL:    "unknown",
				SourceKind: "unknown",
			},
			wantRepo:       "https://argoproj.github.io/argo-helm",
			wantSourceKind: "helm_repo",
		},
		{
			name: "applies when source is git",
			input: model.ChartRecord{
				ChartName:  "vault",
				RepoURL:    "https://github.com/hashicorp/vault",
				SourceKind: "git",
			},
			wantRepo:       "https://helm.releases.hashicorp.com",
			wantSourceKind: "helm_repo",
		},
		{
			name: "leaves authoritative helm_repo alone",
			input: model.ChartRecord{
				ChartName:  "vault",
				RepoURL:    "https://something-else.example.com/charts",
				SourceKind: "helm_repo",
			},
			wantRepo:       "https://something-else.example.com/charts",
			wantSourceKind: "helm_repo",
		},
		{
			name: "no-op when chart not in override map",
			input: model.ChartRecord{
				ChartName:  "policy-reporter",
				RepoURL:    "unknown",
				SourceKind: "unknown",
			},
			wantRepo:       "unknown",
			wantSourceKind: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Builder{repoOverrides: overrides}
			got := b.applyRepoOverride(tc.input)
			if got.RepoURL != tc.wantRepo {
				t.Fatalf("RepoURL = %q, want %q", got.RepoURL, tc.wantRepo)
			}
			if got.SourceKind != tc.wantSourceKind {
				t.Fatalf("SourceKind = %q, want %q", got.SourceKind, tc.wantSourceKind)
			}
		})
	}
}
