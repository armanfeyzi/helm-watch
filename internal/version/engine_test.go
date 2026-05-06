package version

import (
	"testing"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

func TestEngineCompare(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name           string
		current        string
		latest         string
		expectedStatus model.VersionStatus
		expectLagGT0   bool
	}{
		{
			name:           "equal versions are up to date",
			current:        "1.6.0",
			latest:         "1.6.0",
			expectedStatus: model.VersionStatusUpToDate,
			expectLagGT0:   false,
		},
		{
			name:           "older current is outdated",
			current:        "1.6.0",
			latest:         "1.8.2",
			expectedStatus: model.VersionStatusOutdated,
			expectLagGT0:   true,
		},
		{
			name:           "current ahead of latest is treated up to date",
			current:        "2.0.0",
			latest:         "1.9.9",
			expectedStatus: model.VersionStatusUpToDate,
			expectLagGT0:   false,
		},
		{
			name:           "pre release compares correctly",
			current:        "1.8.2-beta.1",
			latest:         "1.8.2",
			expectedStatus: model.VersionStatusOutdated,
			expectLagGT0:   true,
		},
		{
			name:           "invalid current is unknown",
			current:        "not-a-version",
			latest:         "1.8.2",
			expectedStatus: model.VersionStatusUnknown,
			expectLagGT0:   false,
		},
		{
			name:           "invalid latest is unknown",
			current:        "1.8.2",
			latest:         "latest",
			expectedStatus: model.VersionStatusUnknown,
			expectLagGT0:   false,
		},
		{
			name:           "empty latest is unknown",
			current:        "1.8.2",
			latest:         "",
			expectedStatus: model.VersionStatusUnknown,
			expectLagGT0:   false,
		},
		{
			name:           "unknown latest marker is unknown",
			current:        "1.8.2",
			latest:         "unknown",
			expectedStatus: model.VersionStatusUnknown,
			expectLagGT0:   false,
		},
		{
			name:           "leading v prefix is handled",
			current:        "v1.6.0",
			latest:         "1.6.1",
			expectedStatus: model.VersionStatusOutdated,
			expectLagGT0:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Compare(tt.current, tt.latest)
			if result.Status != tt.expectedStatus {
				t.Fatalf("expected status %q, got %q", tt.expectedStatus, result.Status)
			}

			if tt.expectLagGT0 && result.Lag <= 0 {
				t.Fatalf("expected positive lag, got %f", result.Lag)
			}
			if !tt.expectLagGT0 && result.Lag != 0 {
				t.Fatalf("expected zero lag, got %f", result.Lag)
			}
		})
	}
}
