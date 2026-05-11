# Cluster validation (non-production)

Use this checklist after a release build to confirm Helm Watch behaves correctly **in a real Kubernetes cluster**. CI proves compile and unit tests; this proves scrape paths, RBAC, and metrics you will rely on in Grafana.

**Prerequisites:** `kubectl` configured for a **non-production** cluster, optional `helm` 3.x.

## 1) Deploy

Pick **one** path.

### Option A — Helm chart (recommended)

From the repository root, pin an image tag that matches your release (avoid drifting `latest` when validating a specific version):

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch --create-namespace \
  --set image.tag=v0.1.2
```

Replace `v0.1.2` with the tag you are validating.

### Option B — Raw manifests

Edit `deploy/k8s/deployment.yaml` if you need a specific image digest or tag, then:

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/serviceaccount.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
```

Wait for rollout:

```bash
kubectl rollout status deployment/helm-watch -n helm-watch --timeout=120s
```

## 2) Pod health and metrics (port-forward)

```bash
kubectl port-forward -n helm-watch svc/helm-watch 8080:8080
```

In another terminal:

```bash
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/metrics | head -n 40
```

Expect `200` from `/healthz` and Prometheus text exposition from `/metrics` (including `helm_chart_*` or reconcile counters after the first reconcile cycle).

## 3) RBAC and discovery smoke check

Confirm the pod is running and logs show reconcile activity (adjust log level via chart `config.logLevel` if needed):

```bash
kubectl logs -n helm-watch -l app.kubernetes.io/name=helm-watch --tail=50
```

If you see repeated permission errors on Argo CD or Helm release objects, fix `deploy/k8s/rbac.yaml` or chart RBAC for your cluster policies, then re-run.

## 4) Prometheus scrape (Prometheus Operator)

If you use kube-prometheus-stack (or another Prometheus Operator install), align the ServiceMonitor label with your Prometheus CR `serviceMonitorSelector` (see `deploy/monitoring/README.md`).

Enable the chart ServiceMonitor **or** apply the sample:

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch \
  --reuse-values \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.namespace=monitoring
```

or:

```bash
kubectl apply -f deploy/monitoring/servicemonitor.yaml
```

Then in the Prometheus UI (**Status → Targets**), confirm a target for the `helm-watch` service in namespace `helm-watch` is **UP**. In **Graph**, try:

```promql
helm_watch_reconcile_duration_seconds
```

and:

```promql
count(helm_chart_info)
```

## 5) Grafana

Import `deploy/monitoring/grafana-dashboard.json` and select your Prometheus datasource. If panels are empty but Explore shows series, see **Grafana: dashboard panels empty** in `deploy/monitoring/README.md` (namespace variable / `allValue`).

## 6) Optional — alerts

If you use PrometheusRule CRDs:

```bash
kubectl apply -f deploy/monitoring/prometheus-rules.yaml
```

Tune rule filters for your namespace conventions before relying on them in production.

## 7) Record findings

Capture in an issue or `docs/release-verification-v0.1.x.md`:

| Check | Pass / Fail | Notes |
| --- | --- | --- |
| Rollout healthy | | |
| `/healthz` | | |
| `/metrics` exposes `helm_chart_*` | | |
| Share of workloads with `latest_version="unknown"` | | |
| Prometheus target UP | | |
| Grafana panels sane | | |
| Alerts (if enabled) not noisy | | |

Use failures to drive the next patch release (overrides, RBAC, OCI auth, dashboard variables).
