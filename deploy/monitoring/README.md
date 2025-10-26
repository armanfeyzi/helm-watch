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

## Import Grafana dashboard

1. Open Grafana
2. Go to Dashboards -> Import
3. Upload `deploy/monitoring/grafana-dashboard.json`
4. Select your Prometheus datasource (default in file: `uid=prometheus`)

## Notes

- `ServiceMonitor` and `PrometheusRule` require Prometheus Operator CRDs.
- Update namespace labels and alert filters to match your environment.
