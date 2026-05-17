# API Guide

## Transport Surfaces

### Ingest

- `POST /v1/traces`
  OTLP/HTTP trace ingestion.
- `POST /api/v1/spans`
  Native JSON span ingestion.
- `POST /api/v2/spans`
  Zipkin v2 JSON ingestion.

### Query

- `GET /api/v1/traces`
  Trace listing with filtering, pagination, sorting, and attribute matching.
- `GET /api/v1/traces/{traceId}`
  Trace detail including spans and analysis outputs.
- `GET /api/v1/traces/{traceId}/export`
  Export of a trace payload.
- `POST /api/v1/traces/import`
  Import one or more trace payloads.
- `DELETE /api/v1/traces/{traceId}`
  Delete a trace.
- `POST /api/v1/traces/archive`
  Write a snapshot archive of the current trace set.
- `POST /api/v1/traces/archive/restore`
  Restore a trace archive from disk.
- `GET /api/v1/traces/compare?base=...&compare=...`
  Structural comparison between two traces.

### Discovery

- `GET /api/v1/services`
- `GET /api/v1/operations?service=...`
- `GET /api/v1/dependencies`

### Metrics

- `GET /api/v1/metrics/red`
- `GET /api/v1/metrics/heatmap`
- `GET /api/v1/metrics/anomalies`
- `GET /api/v1/metrics/slo`

### Alerting

- `GET /api/v1/alerts`
  Current active alerts derived from readiness, SLO, and anomaly signals.

### Sampler

- `GET /api/v1/sampler`
- `PUT /api/v1/sampler`

### Public Probes and Metadata

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /openapi.yaml`
- `GET /api/v1/config`

## Authentication

When `AUTH_TOKENS` or `API_KEY` is set, protected APIs require:

```http
Authorization: Bearer <token>
```

`AUTH_TOKENS` uses the format:

```text
token|role|tenant;token|role|tenant
```

Roles:

- `viewer`
  Query, metrics, alerts, and SSE.
- `operator`
  Viewer permissions plus ingest and lifecycle operations.
- `admin`
  Operator permissions plus sampler, stats, pprof, and archive restore.

Tenant-aware callers can set `X-Tenant-ID` when using a global admin token to scope requests to a specific tenant. Public probe and metadata endpoints remain unauthenticated.

## SSE Streams

- `GET /sse/traces`
  Trace summary events only.
- `GET /sse/spans`
  Span-level live events only.
- `GET /sse/metrics`
  Metrics refresh tick stream.
- `GET /sse/sampler`
  Sampler stats refresh tick stream.

## Sampler Configuration Notes

The sampler API validates inputs strictly:

- probabilistic `rate` must be in `[0, 1]`
- rate-limit `tracesPerSec` must be positive
- adaptive target must be positive
- adaptive min/max rates must be valid probabilities
- tail policy types must be known
- nested rule samplers must use supported sampler types

## Reference Material

- Machine-readable contract: [`api/openapi.yaml`](../api/openapi.yaml)
- Route wiring: [`api/server.go`](../api/server.go)
- Sampler request mapping: [`api/handler_sampler.go`](../api/handler_sampler.go)
