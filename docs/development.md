# Development Guide

## Local Workflow

### Start Everything

```bash
make dev
make demo
```

### Backend

```bash
go test ./...
go test ./... -race
go test ./... -run TestName
```

### Frontend

```bash
cd web
npm test
npm run test:e2e
npx tsc --noEmit
npm run build
```

## Repository Layout

### Backend

- `cmd/`
  Executables
- `api/`
  HTTP/gRPC ingress and route handlers
- `internal/processor/`
  Span enrichment and trace assembly
- `internal/analysis/`
  Trace analysis logic
- `internal/sampler/`
  Sampling implementations
- `internal/storage/`
  Trace persistence and query logic
- `internal/metrics/`
  RED metrics, histograms, heatmaps, SLOs, anomaly logic

### Frontend

- `web/src/pages/`
  Route-level screens
- `web/src/components/`
  Reusable UI and visualization primitives
- `web/src/hooks/`
  Data-fetch and UX hooks
- `web/src/api/`
  Backend client

## Contribution Expectations

- keep issue scope focused
- add regression tests for correctness bugs
- prefer shared UI patterns over page-specific one-offs
- avoid partial fixes that leave hidden behavior changes untested
- keep route-level UX explicit for loading, error, and empty states

## Benchmark Workflow

```bash
go test ./internal/storage ./internal/metrics ./internal/sampler -run '^$' -bench . -benchmem
```

Use benchmark changes to expose hot-path allocation and concurrency behavior, not just raw throughput.
