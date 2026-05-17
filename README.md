# Distributed Tracing System

A full-stack distributed tracing system with a Go collector backend and React frontend. Features span ingestion (native + OTLP), live metrics, pluggable samplers, and interactive visualization.

## Quick Start

```bash
# 1. Start collector (port 4318) + React dev server (port 5173)
make dev

# 2. In a separate terminal, send synthetic trace traffic
make demo

# 3. Open the UI
open http://localhost:5173
```

That's it. Within ~10 seconds you'll see traces appearing in the Search page.

## Documentation

Detailed project documentation now lives in [`docs/`](./docs/README.md):

- [Architecture](./docs/architecture.md)
- [API Guide](./docs/api.md)
- [Frontend Guide](./docs/frontend.md)
- [Operations Guide](./docs/operations.md)
- [Development Guide](./docs/development.md)
- [Frontend Audit](./docs/frontend-audit.md)
- [Runbooks](./docs/runbooks.md)
- [Release Guide](./docs/release.md)

## Architecture

```
Traffic Generator ──OTLP/HTTP──► Collector (Go)
                               │
                    ┌──────────┼──────────┐
                    │          │          │
                 Sampler   Assembler   Metrics
                    │          │          │
                    └──────────▼──────────┘
                           MemoryStore
                               │
                            HTTP API ──SSE──► React UI
```

**Backend** (`cmd/collector`) — single Go binary:
- OTLP HTTP receiver at `/v1/traces`
- Native JSON receiver at `/api/v1/spans`
- Assembler: groups spans into traces with a 2s quiet-period timer
- Pluggable samplers: Always, Never, Probabilistic, RateLimit, Adaptive, Rule-Based, Tail-Based
- RED metrics (rate, error rate, P50/P95/P99) with sliding-window histograms
- Server-Sent Events bus for live UI updates
- In-memory store with configurable eviction (default 10k traces)
- Static token RBAC with tenant-aware ingest/query isolation
- Optional peer replication and lifecycle/archive APIs for operational workflows

**Frontend** (`web`) — React 18 + TypeScript + Vite:
- **Search** — filter traces by service, operation, duration, error status, time range
- **Trace Detail** — D3 waterfall chart with zoom, minimap, critical path highlighting, span drawer
- **Service Map** — React Flow graph with Dagre layout; node size/color encodes span count/error rate
- **Metrics** — Recharts dashboards (rate, error rate, latency percentiles, heatmap)
- **Sampler** — Live config panel; switch sampler type with confirmation diff dialog
- **Compare** — Side-by-side waterfall diff with delta badges, grayed/green overlay for added/removed spans

## Commands

| Command | Description |
|---|---|
| `make dev` | Start collector + Vite dev server |
| `make demo` | Run synthetic traffic generator |
| `make test` | Run all Go tests |
| `make race` | Run Go tests with `-race` |
| `make build` | Build collector binary to `bin/collector` |
| `make loadtest` | Run the k6 ingest load script |
| `make loadtest-mixed` | Run mixed ingest + query pressure |
| `make soaktest` | Run the longer collector soak profile |
| `make integration` | Run the API integration suite for auth, lifecycle, replication, and alerting |
| `make helm-lint` | Lint and render the Helm chart |
| `make release-dry-run` | Run a local release packaging dry run |
| `cd web && npm test` | Run frontend unit tests |
| `cd web && npx tsc --noEmit` | TypeScript type check |

## Sampler Types

| Type | Description |
|---|---|
| `always` | Sample every trace |
| `never` | Drop all traces |
| `probabilistic` | Hash-based deterministic sampling at configured rate |
| `ratelimit` | Token-bucket; cap at N traces/sec |
| `adaptive` | Adjusts probabilistic rate to hit a target throughput |
| `rules` | Priority-ordered glob rules; per-service and per-operation matching |
| `tail` | Buffer-and-decide: error policy, latency threshold, probabilistic fallback |

Switch sampler live via the UI (Sampler page) or API:

```bash
curl -X PUT http://localhost:4318/api/v1/sampler \
  -H 'Content-Type: application/json' \
  -d '{"type":"probabilistic","rate":0.1}'
```

## API Reference

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/traces` | OTLP HTTP ingest |
| `POST` | `/api/v1/spans` | Native span ingest |
| `GET` | `/api/v1/traces` | Query traces (service, op, error, duration, time range, pagination) |
| `GET` | `/api/v1/traces/:id` | Full trace with spans + critical path analysis |
| `GET` | `/api/v1/traces/compare?base=&compare=` | Diff two traces |
| `GET` | `/api/v1/services` | List known services |
| `GET` | `/api/v1/operations?service=` | List operations for a service |
| `GET` | `/api/v1/dependencies` | Service dependency graph |
| `GET` | `/api/v1/metrics/red` | RED metrics snapshot |
| `GET` | `/api/v1/sampler` | Current sampler config + stats |
| `PUT` | `/api/v1/sampler` | Swap sampler config |
| `GET` | `/sse/traces` | SSE stream: new trace events |
| `GET` | `/sse/metrics` | SSE stream: metrics updates |
| `GET` | `/sse/sampler` | SSE stream: sampler stat updates |

## Deployment Assets

- `docker-compose.yml`
  Local multi-container stack with optional demo traffic and observability profile.
- [`deploy/k8s/`](./deploy/k8s/README.md)
  Kubernetes example manifests for collector and web services.
- [`deploy/helm/tracing/`](./deploy/helm/tracing)
  Packaged Helm chart for Kubernetes installation.
- [`deploy/observability/`](./deploy/observability/README.md)
  Prometheus scrape config and starter Grafana dashboard JSON.
- [`loadtests/`](./loadtests/README.md)
  Load-generation assets for collector ingest pressure testing.
