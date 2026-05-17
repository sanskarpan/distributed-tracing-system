# Operations Guide

## Runtime Configuration

### Core

- `LISTEN_ADDR`
  HTTP listen address. Default `:4318`.
- `GRPC_ADDR`
  gRPC OTLP listen address. Default `:4317`.
- `API_KEY`
  Enables bearer-token protection for protected endpoints.
- `AUTH_TOKENS`
  Static token map in `token|role|tenant;...` form. Enables RBAC and tenant isolation for HTTP and gRPC OTLP ingest.
- `HTTP_READ_HEADER_TIMEOUT`
  HTTP request header timeout. Default `5s`.
- `HTTP_READ_TIMEOUT`
  Total HTTP request read timeout. Default `15s`.
- `HTTP_WRITE_TIMEOUT`
  HTTP response write timeout. Default `30s`.
- `HTTP_IDLE_TIMEOUT`
  Keep-alive idle timeout. Default `60s`.
- `HTTP_MAX_HEADER_BYTES`
  Maximum HTTP header size in bytes. Default `1048576`.
- `READINESS_MAX_QUEUE_USAGE_PCT`
  Optional readiness trip point. When set to a value greater than `0`, `/readyz` returns `503` with status `overloaded` once the worker queue usage reaches or exceeds this percentage.
- `ENABLE_PPROF`
  When set to `true`, enables protected `/debug/pprof/*` endpoints behind the same API key middleware as the rest of the control plane.

### Storage

- `DATA_DIR`
  Enables Badger-backed durable storage.
- `TRACE_TTL`
  Enables retention eviction for the in-memory store.
- `ARCHIVE_DIR`
  Filesystem location for lifecycle archive snapshots.

### TLS

- `TLS_CERT_FILE`
- `TLS_KEY_FILE`

### UI/Integration

- `LOG_LINK_TEMPLATE`
  Used by the frontend when constructing external log links.
- `VITE_API_TOKEN`
  Frontend bearer token for protected API deployments.
- `VITE_TENANT_ID`
  Frontend tenant scope for protected multi-tenant deployments.

## Deployment Notes

### Collector

The collector still runs as a single service process, but it now supports:

Operational characteristics:

- in-memory query path by default
- optional durable persistence with Badger
- HTTP and gRPC receivers in the same process
- graceful shutdown now drains HTTP requests, sampler buffers, worker queues, and pending assembler traces
- tenant-aware ingest, query, alerting, and trace lifecycle operations

### Frontend

The frontend is a static build served separately in development through Vite. In production it can be deployed behind any static host or reverse proxy as long as API routes are proxied to the collector.

### Deployment Examples

- `docker-compose.yml` includes optional `demo` and `observability` profiles.
- `deploy/k8s/` contains a baseline collector + web deployment for Kubernetes.
- `deploy/observability/` contains Prometheus and Grafana starter assets.

## Observability

### Built-in

- `/healthz`
- `/readyz`
- `/metrics`
- `/api/v1/stats`
- `/api/v1/sampler`
- `/api/v1/alerts`
- `/debug/pprof/*` when `ENABLE_PPROF=true`
- frontend live metrics and sampler pages

### Important Signals

- queue depth
- queue saturation percentage
- sampled vs dropped totals
- trace count/max trace count
- service-level rate/error/latency
- anomalous latency spikes
- alert webhook delivery behavior

## Failure Modes to Watch

- high ingest with constrained memory-backed storage
- readiness staying green while the worker queue is saturating
- slow frontend clients causing SSE event drops
- large trace visualizations stressing browser rendering
- sampler misconfiguration producing unexpected throughput

## Debugging Profiles

If `ENABLE_PPROF=true` is enabled in a trusted environment, you can inspect:

- `/debug/pprof/heap`
- `/debug/pprof/goroutine`
- `/debug/pprof/profile`
- `/debug/pprof/trace`

These endpoints are protected by `API_KEY` when authentication is enabled. They should not be exposed publicly without an access boundary.

## Shutdown Semantics

On shutdown:

1. Readiness flips from `ready` to `draining`, so `/readyz` returns `503` and upstream load balancers can stop routing new traffic.
2. HTTP server begins graceful drain.
3. gRPC server stops accepting work.
4. pipeline workers drain accepted spans.
5. deferred samplers flush pending decisions.
6. assembler flushes pending traces to storage.

This behavior is covered by backend tests and was added specifically to avoid dropping in-flight traces during process exit.
