# Operations Guide

## Runtime Configuration

### Core

- `LISTEN_ADDR`
  HTTP listen address. Default `:4318`.
- `GRPC_ADDR`
  gRPC OTLP listen address. Default `:4317`.
- `API_KEY`
  Enables bearer-token protection for protected endpoints.

### Storage

- `DATA_DIR`
  Enables Badger-backed durable storage.
- `TRACE_TTL`
  Enables retention eviction for the in-memory store.

### TLS

- `TLS_CERT_FILE`
- `TLS_KEY_FILE`

### UI/Integration

- `LOG_LINK_TEMPLATE`
  Used by the frontend when constructing external log links.

## Deployment Notes

### Collector

The collector is intended to run as a single service process.

Operational characteristics:

- in-memory query path by default
- optional durable persistence with Badger
- HTTP and gRPC receivers in the same process
- graceful shutdown now drains HTTP requests, sampler buffers, worker queues, and pending assembler traces

### Frontend

The frontend is a static build served separately in development through Vite. In production it can be deployed behind any static host or reverse proxy as long as API routes are proxied to the collector.

## Observability

### Built-in

- `/healthz`
- `/readyz`
- `/metrics`
- `/api/v1/stats`
- `/api/v1/sampler`
- frontend live metrics and sampler pages

### Important Signals

- queue depth
- sampled vs dropped totals
- trace count/max trace count
- service-level rate/error/latency
- anomalous latency spikes

## Failure Modes to Watch

- high ingest with constrained memory-backed storage
- slow frontend clients causing SSE event drops
- large trace visualizations stressing browser rendering
- sampler misconfiguration producing unexpected throughput

## Shutdown Semantics

On shutdown:

1. HTTP server begins graceful drain.
2. gRPC server stops accepting work.
3. pipeline workers drain accepted spans.
4. deferred samplers flush pending decisions.
5. assembler flushes pending traces to storage.

This behavior is covered by backend tests and was added specifically to avoid dropping in-flight traces during process exit.
