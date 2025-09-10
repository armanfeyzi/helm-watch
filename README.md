# Helm Watch

Helm Watch is a Kubernetes observability service that gives teams real-time visibility into Helm chart version drift across deployment methods.

It discovers Helm-based workloads in-cluster, resolves current deployed versions, compares them with upstream chart versions, and exposes the result as Prometheus metrics for Grafana dashboards and alerts.

## Why Helm Watch

In most clusters, Helm charts are deployed through different paths:

- Argo CD (GitOps)
- Helm CLI/manual workflows
- Terraform Helm provider

This fragmentation makes it hard to answer simple but critical questions:

- What chart versions are currently running?
- Which workloads are outdated?
- Where are upgrade risks or security lag accumulating?

Helm Watch addresses this with a single, cluster-side source of truth.

## Core Capabilities (MVP)

- Discover Helm-based workloads from:
  - Argo CD `Application` resources
  - Helm release storage objects (`Secret`/`ConfigMap`, `owner=helm`)
- Normalize workload metadata across sources
- Resolve latest chart versions from upstream Helm repositories
- Compare `current` vs `latest` versions
- Export Prometheus metrics at `/metrics`

## Out of Scope (MVP)

- Automatic upgrades
- Git repository write-back
- Built-in UI (Grafana is the UI)
- Helm deployment lifecycle management

## High-Level Architecture

```text
Kubernetes Cluster
     ↓
[Helm Watch Service]
     ├─ Discovery Layer
     ├─ Metadata Normalization
     ├─ Repository Resolver + Cache
     ├─ Version Comparison Engine
     └─ Metrics Exporter (/metrics)
     ↓
Prometheus
     ↓
Grafana / Alerting
```

## Current Project Status

Implemented so far:

- Service runtime skeleton
- Health and metrics endpoints
- Canonical internal data contracts
- Discovery manager with periodic reconcile loop
- Argo CD + Helm release source adapters
- Repository resolver with cache and stale-on-error behavior
- Version comparison engine
- Core chart metrics pipeline
- Deploy directory with Kubernetes manifests and Helm chart
- GitHub Actions CI producing binaries and container image artifacts

Planned next:

- Repository resolver (`index.yaml` fetch + cache)
- Version comparison engine
- Full metrics contract and dashboards

## Example Metrics

```text
helm_chart_info{app="alloy",namespace="monitoring",chart="alloy",repo="https://grafana.github.io/helm-charts",current_version="1.6.0",latest_version="1.8.2",deployment_type="argocd"} 1
helm_chart_outdated{app="alloy",namespace="monitoring",chart="alloy"} 1
helm_chart_version_lag{app="alloy",namespace="monitoring",chart="alloy"} 202
```

## Run Locally

Prerequisites:

- Go 1.22+ (or compatible recent version)
- Access to a Kubernetes cluster

Run:

```bash
go run ./cmd/helm-watch
```

Useful environment variables:

- `HELM_WATCH_HTTP_ADDR` (default `:8080`)
- `HELM_WATCH_HTTP_READ_TIMEOUT` (default `10s`)
- `HELM_WATCH_HTTP_WRITE_TIMEOUT` (default `10s`)
- `HELM_WATCH_SHUTDOWN_TIMEOUT` (default `10s`)
- `HELM_WATCH_RECONCILE_EVERY` (default `30s`)
- `HELM_WATCH_REPO_CACHE_TTL` (default `5m`)
- `HELM_WATCH_RESOLVE_WORKERS` (default `8`)
- `HELM_WATCH_KUBECONFIG` (optional fallback for local runs)
- `HELM_WATCH_LOG_LEVEL` (default `info`)

Endpoints:

- `GET /healthz`
- `GET /metrics`

## Deploy to Kubernetes

Two deployment methods are included:

- Raw manifests: `deploy/k8s/`
- Helm chart: `deploy/helm-watch/`

See `deploy/README.md` for commands.

## CI Artifacts

GitHub Actions workflow at `.github/workflows/ci.yml`:

- runs tests
- lints Helm chart
- builds Linux binaries
- builds Docker image
- uploads artifacts (binaries + Docker image tar)

Tag-based release workflow at `.github/workflows/release.yml`:

- runs tests on `v*` tags
- builds Linux release binaries (`amd64`, `arm64`)
- lints and packages Helm chart (`.tgz`)
- pushes multi-arch image to GHCR
- creates GitHub Release and uploads binaries, checksums, and chart package

## Roadmap

- Repository resolver and caching strategy
- Version engine with robust semver handling
- Outdated and lag metrics
- Dashboard and alert templates
- Hardening for edge cases (multi-source apps, unsupported sources, partial failures)

## License

Licensed under Apache License 2.0. See `LICENSE` for full terms.
