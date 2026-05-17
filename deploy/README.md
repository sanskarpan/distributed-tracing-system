# Deployment Assets

This directory contains production-oriented examples that complement the local `docker-compose.yml`.

## Layout

- `k8s/`
  Kubernetes manifests for a basic collector + web deployment.
- `helm/`
  Packaged Helm chart for Kubernetes installation and release artifacts.
- `observability/`
  Prometheus scrape configuration and a starter Grafana dashboard.

## Notes

- Replace image references before applying the Kubernetes manifests.
- The Kubernetes examples assume in-cluster DNS names `collector` and `web`.
- The Grafana dashboard targets the Prometheus metrics exposed by `/metrics`.
- For staging or production profiling, keep `ENABLE_PPROF` disabled by default and enable it only inside a trusted network boundary.
- If you use readiness-based queue shedding, set `READINESS_MAX_QUEUE_USAGE_PCT` explicitly so the deployment behavior is predictable under sustained ingest pressure.
