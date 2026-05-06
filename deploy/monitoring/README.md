# Monitoring Bootstrap

This directory provides starter monitoring assets for Helm Watch.

## Included files

- `servicemonitor.yaml` - Prometheus Operator `ServiceMonitor` for scraping `/metrics`
- `prometheus-rules.yaml` - baseline alerting rules
- `grafana-dashboard.json` - ready-to-import Grafana dashboard

## Apply monitoring manifests

```bash
kubectl apply -f deploy/monitoring/servicemonitor.yaml
kubectl apply -f deploy/monitoring/prometheus-rules.yaml
```

If you deploy with Helm chart, you can create ServiceMonitor from chart values instead:

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.namespace=monitoring
```

## Import Grafana dashboard

1. Open Grafana
2. Go to Dashboards -> Import
3. Upload `deploy/monitoring/grafana-dashboard.json`
4. Select your Prometheus datasource when prompted (`DS_PROMETHEUS`)

## Notes

- `ServiceMonitor` and `PrometheusRule` require Prometheus Operator CRDs.
- Update namespace labels and alert filters to match your environment.

## Grafana Explore: no `helm_chart_*` but the pod `/metrics` is fine

Grafana queries **Prometheus**, not the Helm Watch pod. If Prometheus never scraped Helm Watch, those series do not exist in TSDB.

1. Check your Prometheus CR picks up ServiceMonitors:

   ```bash
   kubectl get prometheus -A -o yaml | grep -A5 serviceMonitorSelector
   ```

2. Install a `ServiceMonitor` whose **metadata.labels** match that selector. The sample `servicemonitor.yaml` sets `release: prometheus` — adjust if your stack uses another value (for example `release: kube-prometheus-stack`).

   ```bash
   kubectl apply -f deploy/monitoring/servicemonitor.yaml
   ```

3. In Prometheus UI, confirm a **Targets** entry for the `helm-watch` service (name varies after relabeling).

## Grafana: dashboard panels empty (but Explore works)

Panels filter with `namespace=~"$namespace"`. The `namespace` label is the **workload** namespace (for example `argocd`, `kube-system`), not only where Helm Watch runs. If the variable **All** expands to `$__all`, the regex can match nothing and every panel looks empty.

The bundled dashboard sets `allValue` to `.+` so **All** selects every workload namespace. Re-import `grafana-dashboard.json` after upgrades, or edit the variable: **Show variable** → **Custom all value** → `.+`.
