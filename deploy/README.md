# Deployment Methods

This directory contains two deployment options for Helm Watch:

- `k8s/` - raw Kubernetes manifests
- `helm-watch/` - Helm chart
- `monitoring/` - ServiceMonitor, PrometheusRule, and Grafana dashboard

## Option 1: Raw Kubernetes manifests

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/serviceaccount.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
```

## Option 2: Helm chart

```bash
helm upgrade --install helm-watch ./deploy/helm-watch --namespace helm-watch --create-namespace
```

Enable ServiceMonitor from the chart:

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.namespace=monitoring
```

Install PrometheusRule alerts from the chart (namespace regex, unknown ratio threshold, and durations are under `prometheusRule` in `deploy/helm-watch/values.yaml`):

```bash
helm upgrade --install helm-watch ./deploy/helm-watch \
  --namespace helm-watch --create-namespace \
  --set prometheusRule.enabled=true \
  --set prometheusRule.namespace=monitoring
```

## Notes

- Update image tag/repository for your environment before deploy.
- Service listens on port `8080` and exposes:
  - `/healthz`
  - `/metrics`
- For monitoring assets, see `deploy/monitoring/README.md`.
