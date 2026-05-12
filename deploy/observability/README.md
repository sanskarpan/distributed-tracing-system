# Observability Assets

- `prometheus.yml`
  Scrapes the collector's `/metrics` endpoint.
- `grafana-dashboard.json`
  Starter dashboard for request rate, error rate, latency, and sampler throughput.

The dashboard assumes the Prometheus datasource is named `Prometheus`.
