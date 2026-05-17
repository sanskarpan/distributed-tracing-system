# Architecture

## Overview

The system is a single-process tracing collector with a browser-based analysis UI:

1. Clients send spans through OTLP/HTTP, native JSON, or Zipkin JSON.
2. The collector validates spans, applies sampling, enriches metadata, records metrics, and assembles traces.
3. Completed traces are stored in memory or in Badger-backed durable storage.
4. Optional peer replication can fan native ingest batches to additional collectors.
5. Query APIs expose traces, metrics, alerts, dependency graphs, lifecycle operations, and sampler state.
6. The frontend consumes APIs directly and uses SSE for live updates.

## Runtime Topology

```text
apps / demo traffic
    |
    +--> OTLP HTTP     POST /v1/traces
    +--> Native JSON   POST /api/v1/spans
    +--> Zipkin JSON   POST /api/v2/spans
                |
                v
         auth + tenant context
                |
                v
         api.Pipeline
           |- sampler
           |- enricher
           |- metrics store
           |- SSE bus
           `- assembler
                |
                v
        analysis + storage + lifecycle
                |
                v
   query / metrics / alerts / lifecycle APIs
                |
                v
            React frontend
```

## Backend Layers

### Entry Points

- `cmd/collector/main.go`
  Main production binary. Sets up storage, metrics, pipeline, HTTP routes, gRPC OTLP receiver, and graceful shutdown.
- `cmd/demo/main.go`
  Synthetic traffic generator used for local exploration and smoke testing.

### API Layer

- `api/server.go`
  Route composition, public vs protected endpoints, live SSE feeds, and periodic sampler/metrics broadcast ticks.
- `api/auth.go`
  Static token parsing, principal context propagation, RBAC, and tenant scoping.
- `api/alerts.go`
  Alert evaluation and webhook delivery.
- `api/lifecycle.go`
  Import, delete, archive, and restore handlers.
- `api/replication.go`
  Async peer fan-out for native span replication.
- `api/handler_*.go`
  Endpoint-level request parsing and response shaping.
- `api/pipeline.go`
  Hot path for ingest: validation, head/tail sampling integration, worker-pool processing, metrics record, SSE span events, and trace assembly.

### Processing Layer

- `internal/processor/enricher.go`
  Span-level normalization/enrichment.
- `internal/processor/assembler.go`
  Quiet-period trace assembly. Builds the trace tree, computes duration, roots, services, and error aggregation.

### Analysis Layer

- `internal/analysis/critical_path.go`
  Critical-path and gap analysis.
- `internal/analysis/dependency.go`
  Service dependency graph generation.
- `internal/analysis/compare.go`
  Structural trace comparison using ancestry-aware span matching.

### Sampling Layer

- `internal/sampler/always.go`
- `internal/sampler/probabilistic.go`
- `internal/sampler/ratelimit.go`
- `internal/sampler/adaptive.go`
- `internal/sampler/rules.go`
- `internal/sampler/tail.go`

The collector now keeps head-sampling decisions trace-consistent in the pipeline so a trace is not partially dropped across spans.

### Storage Layer

- `internal/storage/memory.go`
  Main query and listing implementation.
- `internal/storage/badger.go`
  Durable write-through store backed by Badger while preserving in-memory query behavior.

The store layer is now tenant-aware and supports destructive lifecycle operations required for archive/delete flows.

## Frontend Layers

### Routing

- `web/src/App.tsx`
  Route shell and lazy-loaded heavy routes.
- `web/src/components/Nav.tsx`
  Global navigation and theme toggle.

### Data Access

- `web/src/api/client.ts`
  Fetch wrapper around backend APIs, including optional auth and tenant headers for protected deployments.
- `web/src/hooks/useSearch.ts`
  Search query orchestration, cancellation, pagination merge, and filter state.
- `web/src/hooks/useSSE.ts`
  Lightweight SSE subscription helper.

### Pages

- `Search`
  Live trace listing and filters.
- `TraceDetail`
  Waterfall/flame graph investigation.
- `ServiceMap`
  Dependency visualization.
- `Metrics`
  RED metrics, heatmaps, anomalies, and SLO state.
- `Sampler`
  Live sampler inspection and reconfiguration.
- `Compare`
  Trace-vs-trace diff view.
- `Timeline`
  Time-axis overview of recent traces.

## Data Flow

### Ingest Flow

1. Request handler decodes spans.
2. `Pipeline.IngestSpans` validates identifiers.
3. Tail sampling buffers spans immediately; head samplers use one cached decision per trace.
4. Accepted spans are processed inline or through the worker queue.
5. Processing records metrics, emits span SSE, and passes spans to the assembler.
6. On quiet-period completion, the assembler finalizes the trace.
7. Completed traces run analysis, store persistence, and trace SSE broadcast.

### Query Flow

1. Frontend fetches list/detail/metrics endpoints.
2. Backend handlers query memory/Badger-backed storage or metrics stores.
3. UI merges SSE-driven freshness with query-driven snapshots where appropriate.

## Design Constraints

- Single-binary deployment keeps operational complexity low.
- In-memory-first query semantics optimize local exploration and testability.
- SSE is favored over heavier websocket infrastructure.
- Sampling and trace assembly are designed to preserve trace-level integrity.

## Known Tradeoffs

- Replication is fan-out based rather than a consensus-backed cluster.
- Live frontend state is still page-centric, not globally normalized.
- Some visualization pages remain rich-client heavy despite route-level code splitting.
