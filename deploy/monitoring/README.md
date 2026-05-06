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
