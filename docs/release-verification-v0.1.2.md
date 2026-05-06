# Release Verification - v0.1.2

This document captures Milestone 8 verification status for the first operational release line.

## Release Metadata

- Tag: `v0.1.2`
- Release URL: [v0.1.2 release page](https://github.com/armanfeyzi/helm-watch/releases/tag/v0.1.2)
- Published: 2026-05-06

## CI and Release Pipeline Status

- CI (`main`): success
- CI (`v0.1.2`): success
- Release (`v0.1.2`): success

Verified artifacts in GitHub Release:

- `helm-watch-linux-amd64`
- `helm-watch-linux-arm64`
- `checksums.txt`
- `helm-watch-0.1.0.tgz` (Helm chart package)

## Product Criteria Verification

## 1) Build and test baseline

- `go test ./...` passes in CI.
- Multi-arch image build and GHCR push pass in Release workflow.
- Helm chart lint and package step pass in Release workflow.

Status: ✅ verified

## 2) Deployment packaging

- Raw Kubernetes manifests available under `deploy/k8s/`.
- Helm chart available under `deploy/helm-watch/`.

Status: ✅ verified

## 3) Monitoring bootstrap

- `ServiceMonitor` and `PrometheusRule` manifests available under `deploy/monitoring/`.
- Grafana dashboard JSON available and importable.

Status: ✅ verified (artifact presence)

## 4) Runtime metrics contract

Implemented metrics:

- `helm_chart_info`
- `helm_chart_outdated`
- `helm_chart_version_lag`
- `helm_watch_reconcile_duration_seconds`
- `helm_watch_reconcile_errors_total`

Status: ✅ verified (implementation + tests/build pass)

## 5) Real-cluster acceptance checks (manual)

The following require execution in a live cluster and are not fully automatable in CI:

- Detection coverage target (>=95% Helm workloads)
- End-to-end scrape validation in your Prometheus instance
- Dashboard panel correctness against real data
- Alert firing behavior in production-like namespaces
- Performance under 100+ workloads

Status: ⏳ pending manual environment validation

## Known Limitations (Current Release)

- Native Helm Secret/ConfigMap sources are best-effort and may not always expose complete repo/version metadata.
- OCI-backed chart repos are currently treated as unsupported for upstream latest-version resolution.
- Version lag is a numeric heuristic designed for prioritization, not strict semantic distance.

## Recommended Next Actions

1. Deploy `v0.1.2` to a non-production cluster.
2. Confirm `/healthz` and `/metrics` via port-forward.
3. Validate Prometheus scrape target and dashboard data.
4. Tune alert filters for your actual production namespace naming.
5. Record findings and create `v0.1.3` hardening tasks.
