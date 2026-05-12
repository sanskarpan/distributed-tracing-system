# Deployment Assets

This directory contains production-oriented examples that complement the local `docker-compose.yml`.

## Layout

- `k8s/`
  Kubernetes manifests for a basic collector + web deployment.
- `observability/`
  Prometheus scrape configuration and a starter Grafana dashboard.

## Notes

- Replace image references before applying the Kubernetes manifests.
- The Kubernetes examples assume in-cluster DNS names `collector` and `web`.
- The Grafana dashboard targets the Prometheus metrics exposed by `/metrics`.
