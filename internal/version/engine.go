package version

import (
	"strings"

	semver "github.com/Masterminds/semver/v3"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

type Result struct {
	Status model.VersionStatus
	Lag    float64
}

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Compare(current, latest string) Result {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)

	if current == "" || latest == "" || latest == "unknown" {
		return Result{Status: model.VersionStatusUnknown, Lag: 0}
	}

	cur, err := parseVersion(current)
	if err != nil {
		return Result{Status: model.VersionStatusUnknown, Lag: 0}
	}

	lat, err := parseVersion(latest)
	if err != nil {
		return Result{Status: model.VersionStatusUnknown, Lag: 0}
	}

	if cur.Equal(lat) {
		return Result{Status: model.VersionStatusUpToDate, Lag: 0}
	}

	if cur.GreaterThan(lat) {
		// Current is ahead of latest known upstream entry (rare but possible).
		return Result{Status: model.VersionStatusUpToDate, Lag: 0}
	}

	return Result{
		Status: model.VersionStatusOutdated,
		Lag:    computeLag(cur, lat),
	}
}

func parseVersion(raw string) (*semver.Version, error) {
	raw = strings.TrimSpace(raw)
	// Helm charts often omit a leading v. CoerceVersion helps parse pragmatic inputs.
	return semver.NewVersion(raw)
}

func computeLag(current, latest *semver.Version) float64 {
	majorDiff := latest.Major() - current.Major()
	minorDiff := latest.Minor() - current.Minor()
	patchDiff := latest.Patch() - current.Patch()

	lag := float64(majorDiff*10000 + minorDiff*100 + patchDiff)
	if lag < 1 {
		// Ensure outdated records still produce a positive lag.
		return 1
	}
	return lag
}
