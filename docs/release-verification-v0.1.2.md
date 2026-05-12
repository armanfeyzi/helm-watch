# Release Verification - v0.1.2

> Superseded for current release tracking by [release-verification-v0.2.0.md](./release-verification-v0.2.0.md). This file remains for the v0.1.2 line.

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
- `helm_chart_unknown`
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

**Step-by-step:** follow [cluster-validation.md](./cluster-validation.md) (deploy → port-forward → Prometheus → Grafana → notes).

Status: ⏳ pending manual environment validation (run the checklist above and update this section with date + cluster profile when done)

## Known Limitations (Current Release)

- Native Helm Secret/ConfigMap sources are best-effort and may not always expose complete repo/version metadata.
- **OCI:** public registries are resolved via the Registry HTTP API (anonymous where allowed). **Private** OCI registries need auth that is not fully covered in this release line; use overrides toward a public index when possible, or track auth as a hardening item.
- Version lag is a numeric heuristic designed for prioritization, not strict semantic distance.

## Recommended Next Actions

1. Run [cluster-validation.md](./cluster-validation.md) on a non-production cluster against the image tag you care about (for example `v0.2.0`; see [release-verification-v0.2.0.md](./release-verification-v0.2.0.md) for the current line).
2. Fill in the findings table at the end of that doc (or paste results here under section 5).
3. Tune alert filters and ServiceMonitor labels for your Prometheus Operator install.
4. Open targeted issues for gaps (RBAC, `unknown` rate, private OCI, dashboard variables) and cut `v0.1.3` (or the next patch) from that list.
