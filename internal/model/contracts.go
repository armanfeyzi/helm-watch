package model

import "time"

type DeploymentType string

const (
	DeploymentTypeArgoCD    DeploymentType = "argocd"
	DeploymentTypeHelm      DeploymentType = "helm"
	DeploymentTypeTerraform DeploymentType = "terraform"
	DeploymentTypeUnknown   DeploymentType = "unknown"
)

type SourceType string

const (
	SourceTypeArgoCDApplication SourceType = "argocd_application"
	SourceTypeHelmReleaseSecret SourceType = "helm_release_secret"
	SourceTypeHelmReleaseCM     SourceType = "helm_release_configmap"
)

type VersionStatus string

const (
	VersionStatusUpToDate VersionStatus = "up_to_date"
	VersionStatusOutdated VersionStatus = "outdated"
	VersionStatusUnknown  VersionStatus = "unknown"
)

// WorkloadRecord is the canonical discovered workload contract for Helm Watch.
//
// Namespace is the workload destination namespace (where the application is
// actually deployed). SourceNamespace is where the originating object lives
// — for Argo CD that is the namespace of the Application CR (e.g. argocd),
// for Helm releases it matches the destination. SourceNamespace is used
// internally to re-fetch the source object during enrichment, while
// Namespace is exposed in metrics and dashboards.
type WorkloadRecord struct {
	ID              string
	AppName         string
	Namespace       string
	SourceNamespace string
	SourceType      SourceType
	DeploymentType  DeploymentType
	DetectedAt      time.Time
}

// ChartRecord is the canonical chart view generated for each workload.
type ChartRecord struct {
	WorkloadID      string
	ChartName       string
	RepoURL         string
	SourceKind      string
	CurrentVersion  string
	LatestVersion   string
	Status          VersionStatus
	ResolutionError string
}

type RepoCacheEntry struct {
	RepoURL   string
	FetchedAt time.Time
	ExpiresAt time.Time
	Charts    map[string][]string
	LastError string
}
