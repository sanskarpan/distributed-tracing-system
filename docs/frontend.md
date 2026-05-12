# Frontend Guide

## Goals

The frontend is designed as an investigative UI for live distributed tracing data. It balances:

- fast feedback for demo/local workflows
- dense operational detail for trace debugging
- lightweight live refresh using SSE

## Stack

- React
- TypeScript
- Vite
- Tailwind CSS
- D3 for custom trace/time visualizations
- Recharts for metrics charts
- React Flow + Dagre for the dependency graph

## Route Model

- `/`
  Search and live trace list
- `/trace/:id`
  Single-trace investigation
- `/timeline`
  Trace timeline overview
- `/map`
  Service dependency graph
- `/metrics`
  RED metrics, anomalies, SLOs, heatmap
- `/sampler`
  Sampler control surface
- `/compare`
  Trace diff view
- `*`
  Not-found fallback

Heavy pages are route-split with `React.lazy` to keep initial bundle size down.

## Data Strategy

### Query + SSE Pattern

Most pages use a hybrid approach:

- fetch a consistent snapshot from HTTP APIs
- subscribe to SSE when live refresh is useful
- selectively re-fetch rather than fully modeling backend state on the client

### Search State

`useSearch` owns:

- filter state
- debounced querying
- request cancellation
- pagination merge
- stale response protection

### Global Shared State

`tracingStore` is intentionally small and only stores lightweight cross-page state such as live traces and service lists.

## UI Patterns

The codebase now standardizes on:

- explicit loading states
- explicit empty states
- explicit error states with retry actions
- keyboard-safe interactive controls
- route-level not-found behavior

## Accessibility Expectations

- clickable cards should be keyboard focusable
- sortable tables should use buttons, not click-only table headers
- route navigation should expose active state
- dismiss buttons should have labels
- interactive filters should expose labels or `aria-label`s

## Frontend Risks

- Visualization-heavy pages still rely on imperative libraries and therefore have a higher regression surface than simpler CRUD-style views.
- D3 and React Flow rendering remain harder to unit test than list/detail views.
- A global error boundary is still absent; page-level error handling is currently the primary safety net.
