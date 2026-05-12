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

For **tunable** `PrometheusRule` alerts (namespace regex, unknown ratio threshold, `for` durations), install or upgrade with `prometheusRule.enabled=true` and edit `prometheusRule` in `deploy/helm-watch/values.yaml` (or pass `--set` / `--set-file`). The static `prometheus-rules.yaml` here mirrors the chart defaults and is meant for non-Helm installs.

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch --create-namespace \
  --set prometheusRule.enabled=true \
  --set prometheusRule.namespace=monitoring
```

## Import Grafana dashboard

1. Open Grafana
2. Go to Dashboards -> Import
3. Upload `deploy/monitoring/grafana-dashboard.json`
4. Select your Prometheus datasource when prompted (`DS_PROMETHEUS`)

## Notes

- `ServiceMonitor` and `PrometheusRule` require Prometheus Operator CRDs.
- **kube-prometheus-stack:** your `Prometheus` CR `serviceMonitorSelector` / `ruleSelector` must match labels on these objects. The samples use `release: prometheus`; many installs use `release: kube-prometheus-stack` (or your Helm release name). Edit labels accordingly, or set `serviceMonitor.additionalLabels` / `prometheusRule.additionalLabels` in the chart.
- **`honorLabels`:** the sample `servicemonitor.yaml` and Helm chart (default `serviceMonitor.honorLabels: true`) set `honorLabels: true` on the scrape endpoint. If this is `false`, Prometheus relabeling overwrites the workload `namespace` label on `helm_chart_*` with the **helm-watch pod namespace**, so Grafana tables like “by namespace” collapse to a single row.
- Update namespace labels and alert filters to match your environment.
- The default rules include `HelmWatchUnknownVersionRatioHigh` (>30% unknown versions for 20m in `prod|production` namespaces). Adjust the namespace regex and threshold to your conventions, or use the Helm chart (`prometheusRule` in `deploy/helm-watch/values.yaml`) to template those values per environment.
- The dashboard includes `Unknown Charts` and `Unknown Charts by Namespace` panels based on `helm_chart_unknown`.

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
