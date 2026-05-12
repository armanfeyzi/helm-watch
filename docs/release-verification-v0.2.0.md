# Release Verification - v0.2.0

This document captures verification status for the `v0.2.0` release line. For the previous line, see [release-verification-v0.1.2.md](./release-verification-v0.1.2.md).

## Release Metadata

- Tag: `v0.2.0`
- Release URL: [v0.2.0 release page](https://github.com/armanfeyzi/helm-watch/releases/tag/v0.2.0)
- Published: 2026-05-09

## CI and Release Pipeline Status

- CI (`main`): success (at time of tag)
- CI (`v0.2.0`): success
- Release (`v0.2.0`): success

Verified artifacts in GitHub Release (names follow the workflow; chart package version matches `deploy/helm-watch/Chart.yaml` at release time):

- `helm-watch-linux-amd64`
- `helm-watch-linux-arm64`
- `checksums.txt`
- Packaged Helm chart (`.tgz`)

## Product Criteria Verification

## 1) Build and test baseline

- `go test ./...` passes in CI.
- Multi-arch image build and GHCR push pass in Release workflow.
- Helm chart lint and package step pass in Release workflow.

Status: verified

## 2) Deployment packaging

- Raw Kubernetes manifests available under `deploy/k8s/`.
- Helm chart available under `deploy/helm-watch/`.

Status: verified

## 3) Monitoring bootstrap

- `ServiceMonitor` and `PrometheusRule` manifests available under `deploy/monitoring/`.
- Grafana dashboard JSON available and importable.

Status: verified (artifact presence)

## 4) Runtime metrics contract

Implemented metrics:

- `helm_chart_info`
- `helm_chart_outdated`
- `helm_chart_version_lag`
- `helm_chart_unknown`
- `helm_watch_reconcile_duration_seconds`
- `helm_watch_reconcile_errors_total`

Status: verified (implementation + tests/build pass)

## 5) Real-cluster acceptance checks (manual)

**Step-by-step:** [cluster-validation.md](./cluster-validation.md).

Status: verified (2026-05-12, non-production cluster)

Summary from validation:

- `/healthz` returns `{"status":"ok"}`.
- `/metrics` exposes standard Go and application series; reconcile cycles complete successfully.
- Pod logs show stable discovery and metrics reconciles (example profile: 39 workloads, 39 chart records; reconcile durations on the order of a few to tens of seconds depending on upstream resolution).
- `deploy/monitoring/servicemonitor.yaml` applies cleanly against an existing Prometheus Operator install (`unchanged` when already present).

Operational note: on clusters with many Argo CD `Application` objects, client-go may log **client-side throttling** while listing applications. That is expected Kubernetes behavior under load; if it becomes problematic, tune QPS/burst for the controller (future hardening) or reconcile intervals.

| Check | Pass / Fail | Notes |
| --- | --- | --- |
| Rollout healthy | Pass | |
| `/healthz` | Pass | |
| `/metrics` exposes `helm_chart_*` | Pass | After reconcile |
| Share of workloads with `latest_version="unknown"` | Pass | Acceptable for cluster profile under test |
| Prometheus target UP | Pass | ServiceMonitor applied |
| Grafana panels sane | Pass | Spot-check as applicable |
| Alerts (if enabled) not noisy | Pass | As tuned |

## Known Limitations (Current Release)

- Native Helm Secret/ConfigMap sources are best-effort and may not always expose complete repo/version metadata.
- **OCI:** public registries are resolved via the Registry HTTP API (anonymous where allowed). **Private** OCI registries need auth that is not fully covered in this release line; use overrides toward a public index when possible, or track auth as a hardening item.
- Version lag is a numeric heuristic designed for prioritization, not strict semantic distance.

## Recommended Next Actions

1. Re-run [cluster-validation.md](./cluster-validation.md) when cutting the next tag or after material RBAC/chart changes.
2. Tune alert filters and ServiceMonitor labels for your Prometheus Operator install.
3. Track hardening work (private OCI auth, Argo list QPS/burst, dashboard variables) in issues and the [README roadmap](../README.md#roadmap).
