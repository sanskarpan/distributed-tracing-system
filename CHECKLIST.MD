# Implementation Checklist

Strict phase order. `go test ./... -race` must pass at end of every phase before proceeding.

---

## Phase 0 — Scaffolding

### Backend
- [x] `go mod init github.com/yourname/tracing`
- [x] `go get github.com/go-chi/chi/v5 github.com/stretchr/testify`
- [x] Create full directory tree from SPEC.md
- [x] `cmd/collector/main.go`: stub that prints "collector starting on :4318"
- [x] `cmd/demo/main.go`: stub that prints "demo services starting"
- [x] `Makefile`: `build`, `test`, `race`, `run`, `demo`, `dev`, `lint`

### Frontend
- [x] `cd web && npm create vite@latest . -- --template react-ts`
- [x] Install: `d3 @types/d3 @xyflow/react @dagrejs/dagre @dagrejs/graphlib recharts framer-motion zustand ky tailwindcss @tailwindcss/vite`
- [x] `npx shadcn@latest init`
- [x] Add shadcn: `button badge card tabs select slider switch separator scroll-area tooltip sheet dialog dropdown-menu`
- [x] `vite.config.ts`: proxy `/api`, `/sse`, `/v1` → `http://localhost:4318`
- [x] Verify `npm run dev` runs clean

---

## Phase 1 — IDs and Core Model

- [x] Implement `TraceID [16]byte` with `String()`, `IsZero()`, `MarshalJSON()`, `UnmarshalJSON()`
- [x] Implement `SpanID [8]byte` with same methods
- [x] Implement `NewTraceID() (TraceID, error)` using `crypto/rand`
- [x] Implement `NewSpanID() (SpanID, error)` using `crypto/rand`
- [x] Implement `ParseTraceID(s string) (TraceID, error)`: validate exactly 32 hex chars
- [x] Implement `ParseSpanID(s string) (SpanID, error)`: validate exactly 16 hex chars
- [x] Define `SpanKind`, `StatusCode`, `SpanStatus`, `SpanEvent`, `SpanLink`
- [x] Define `KeyValue` with all value types and constructor functions
- [x] Implement `Span` struct with all fields from SPEC.md
- [x] Implement `Span.Duration()`, `Span.IsRoot()`, `Span.GetAttribute(key)`
- [x] Implement `Trace` struct with all fields
- [x] Write model tests:
  - [x] `NewTraceID()` generates unique IDs (test 1000 calls, no collision)
  - [x] `ParseTraceID` roundtrip: new → string → parse = same
  - [x] `ParseTraceID` rejects 31-char, 33-char, non-hex strings
  - [x] `Span.IsRoot()` true when ParentSpanID is zero
  - [x] JSON marshal/unmarshal roundtrip for Span (IDs as hex strings)

---

## Phase 2 — Propagation

- [x] Define `SpanContext` struct with `IsValid()` method
- [x] Define `Propagator` interface: `Inject(SpanContext, http.Header)`, `Extract(http.Header) (SpanContext, bool)`
- [x] Implement `W3CPropagator`:
  - [x] `Inject`: write `traceparent: 00-{traceID}-{spanID}-{01|00}` + optional `tracestate`
  - [x] `Extract`: parse `traceparent`, validate version `00`, parse IDs, extract sampled flag
  - [x] Reject malformed values (wrong version, wrong ID lengths) → return `false`
- [x] Implement `B3Propagator` (multi-header):
  - [x] `Inject`: write `X-B3-TraceId`, `X-B3-SpanId`, `X-B3-ParentSpanId`, `X-B3-Sampled`
  - [x] `Extract`: read all four headers, parse IDs
- [x] Implement `B3SinglePropagator`:
  - [x] Format: `b3: {traceID}-{spanID}-{1|0}-{parentSpanID}`
- [x] Implement `CompositePropagator`: try W3C first, then B3, then B3-single
- [x] Write propagation tests:
  - [x] W3C inject then extract: SpanContext identical
  - [x] W3C sampled flag: `01` in header → `IsSampled=true`
  - [x] W3C invalid traceparent → `false, SpanContext{}`
  - [x] B3 inject then extract: SpanContext identical
  - [x] Composite: W3C headers take priority over B3 headers
  - [x] Missing headers → `false`
  - [x] Fuzz: random header values don't panic

---

## Phase 3 — Sampler: Always, Never, Probabilistic

- [x] Define `Sampler` interface, `SamplingDecision` enum, `SamplingResult`, `SamplingParameters`
- [x] Implement `AlwaysSampler`: always returns `Sample`
- [x] Implement `NeverSampler`: always returns `Drop`
- [x] Implement `ProbabilisticSampler`:
  - [x] Constructor `NewProbabilistic(rate float64) *ProbabilisticSampler`
  - [x] Compute `threshold = uint64(rate * math.MaxUint64)` on creation and on `SetRate`
  - [x] `ShouldSample`: `id = binary.BigEndian.Uint64(traceID[:8]); return id < threshold`
  - [x] `SetRate(rate)`: update rate + threshold under write lock
  - [x] `Config()`: return `{"rate": 0.1}`
- [x] Write probabilistic sampler tests (CRITICAL):
  - [x] At rate=1.0: 10000 samples → all sampled
  - [x] At rate=0.0: 10000 samples → none sampled
  - [x] At rate=0.1: 10000 samples → approximately 1000 ± 200 sampled
  - [x] **Consistency test**: same traceID + rate=0.1 always produces same decision
  - [x] **Cross-service consistency**: if service A and service B use same traceID and same rate, they ALWAYS agree (simulate with same traceID input, verify same output)
  - [x] Thread-safety: 100 goroutines calling ShouldSample concurrently → `-race` clean

---

## Phase 4 — Sampler: RateLimit and Parent-Based

- [x] Implement `RateLimitSampler` (token bucket):
  - [x] Fields: `tracesPerSec`, `tokens`, `maxTokens = tracesPerSec*2`, `lastRefill`, mutex
  - [x] Refill logic: `tokens += elapsed.Seconds() * tracesPerSec`, cap at `maxTokens`
  - [x] Grant if `tokens >= 1.0`, else drop
- [x] Implement `ParentBasedSampler`:
  - [x] Root spans (ParentSpanID.IsZero()): delegate to `root` sampler
  - [x] Remote parent sampled (`*ParentSampled == true`): delegate to `remoteParentSampled`
  - [x] Remote parent not sampled: delegate to `remoteParentNotSampled`
  - [x] No parent info: delegate to `root`
- [x] Write tests:
  - [x] RateLimit at 10/sec: 50 requests in burst → first 20 sampled (burst=2x), rest dropped
  - [x] RateLimit refills over time: after 1 second, tokens replenished
  - [x] ParentBased: root with AlwaysSampler → sampled; root with NeverSampler → dropped
  - [x] ParentBased: parent sampled=true → AlwaysSampler decision applied
  - [x] ParentBased: parent sampled=false → NeverSampler decision applied

---

## Phase 5 — Sampler: Adaptive and Rule-Based

- [x] Implement `SlidingWindow`: 60 1-second buckets, `Add(n)`, `Rate() float64`
- [x] Implement `AdaptiveSampler`:
  - [x] Wraps `ProbabilisticSampler`
  - [x] Background goroutine calls `adjust()` every `adjustPeriod` (default 5s)
  - [x] `adjust()`: compute `ratio = target / observed`; if `|ratio-1| < 0.10`: skip (hysteresis); else apply `newRate = currentRate * ratio`, clamp to [minRate, maxRate]
  - [x] `Stop()` method to shut down goroutine
- [x] Implement `RuleBasedSampler`:
  - [x] `Rule` struct with all fields from SPEC.md
  - [x] Glob matching for `OperationGlob` (use `path.Match` or simple `*` wildcard)
  - [x] Rules sorted by priority descending at construction
  - [x] Match loop: first matching rule wins; fallback sampler if none match
- [x] Write tests:
  - [x] Adaptive: at 100/s observed, target=100 → rate unchanged
  - [x] Adaptive: at 200/s observed, target=100 → rate halved (after hysteresis)
  - [x] Adaptive: hysteresis — single deviation doesn't trigger adjustment
  - [x] Rules: high-priority error rule matches before low-priority drop rule
  - [x] Rules: glob `HTTP GET *` matches `HTTP GET /api/users`
  - [x] Rules: service filter limits match to correct service
  - [x] Rules: no match → fallback sampler decision used

---

## Phase 6 — Tail-Based Sampler

- [x] Implement `TailPolicy` interface: `Name() string`, `Evaluate(spans []*Span, duration time.Duration) (bool, string)`
- [x] Implement `ErrorPolicy`: keep if any span has `Status.Code == StatusError`
- [x] Implement `LatencyPolicy`: keep if `duration >= threshold`
- [x] Implement `ProbabilisticTailPolicy`: TraceID-hash-based (wraps ProbabilisticSampler logic)
- [x] Implement `TailSampler`:
  - [x] `AddSpan(span *Span)`: add to buffer for its traceID; start timer if new trace
  - [x] On timer fire: evaluate policy chain, call `accept` or `reject` callback
  - [x] `maxSize` eviction: if buffer exceeds limit, evict oldest pending trace (probabilistic drop)
  - [x] `Stop()`: flush all buffered traces (accept or reject based on policies)
- [x] Write tail sampler tests:
  - [x] Error span in trace → ErrorPolicy keeps trace
  - [x] No error, duration < threshold → LatencyPolicy drops; probabilistic fallback decides
  - [x] Trace timeout: after `timeout` seconds without completion → evaluated
  - [x] Under maxSize pressure: oldest buffered trace evicted
  - [x] Accept callback receives all spans for accepted trace
  - [x] Reject callback called for dropped traces
  - [x] Stop() flushes all pending

---

## Phase 7 — Ingestion Pipeline

- [x] Implement `Pipeline` struct: `sampler Sampler`, `assembler *Assembler`, `store Store`, `metrics *MetricsStore`, `broadcast chan SSEEvent`
- [x] Implement `Pipeline.IngestSpans(spans []*Span) error`:
  - [x] For each span: validate (non-zero IDs, valid times)
  - [x] Call `sampler.ShouldSample(params)`
  - [x] If `Drop`: increment dropped counter, skip
  - [x] If `Sample`: enrich span (set ReceivedAt, compute HasError from status)
  - [x] Call `assembler.AddSpan(span)` → may complete a trace
  - [x] For completed traces: run analysis, store, broadcast SSE event
  - [x] Update metrics
- [x] Implement `Assembler`:
  - [x] `pending map[TraceID][]*Span`: spans received before trace complete
  - [x] `AddSpan(span)`: add to pending; if root span received AND all expected children present (or timeout): return complete Trace
  - [x] In practice: complete when we've seen the root span + N seconds of quiet (no new spans for same traceID) — use a timer per traceID
  - [x] Default completion timeout: 2 seconds after last span for same traceID
  - [x] `BuildTree(spans []*Span) *Trace`: construct parent-child relationships
- [x] Implement `Enricher.Enrich(span *Span)`:
  - [x] Set `HasError = span.Status.Code == StatusError`
  - [x] Set `ReceivedAt = time.Now()`
  - [x] Extract `ServiceName` from `ServiceAttrs["service.name"]` if not already set
- [x] Write pipeline tests:
  - [x] Single span ingested → appears in store
  - [x] Span dropped by sampler → not in store, dropped counter incremented
  - [x] Multi-span trace assembled correctly (parent-child links)
  - [x] Orphan span (no parent found within timeout) → stored as incomplete trace
  - [x] Error span → trace.HasError = true, error counter incremented
  - [x] Out-of-order spans (child arrives before parent) → correctly assembled

---

## Phase 8 — HTTP Receivers

- [x] Implement `NativeHTTPReceiver`: `POST /api/v1/spans`
  - [x] Parse JSON body into `[]SpanDTO`
  - [x] Convert DTOs to `[]*model.Span` (parse IDs, nanosecond timestamps)
  - [x] Call `pipeline.IngestSpans(spans)`
  - [x] Return 200 `{"accepted": N, "dropped": M}`
  - [x] Return 400 on parse error with field-level detail
- [x] Implement `OTLPHTTPReceiver`: `POST /v1/traces`
  - [x] Parse OTLP ExportTraceServiceRequest JSON
  - [x] Map `resourceSpans[].resource.attributes["service.name"]` → `span.ServiceName`
  - [x] Map `scopeSpans[].spans[]` fields to `model.Span`
  - [x] nanosecond timestamps: `startTimeUnixNano / 1e6 → ms`, store as `time.Time`
  - [x] Map OTLP attribute types to `KeyValue`
  - [x] Call `pipeline.IngestSpans(spans)`
  - [x] Return `{}` (OTLP spec)
- [x] Write receiver tests with `httptest`:
  - [x] Valid native batch → 200 with correct accepted count
  - [x] Invalid JSON → 400
  - [x] Missing required fields (traceId, spanId) → 400 with details
  - [x] Valid OTLP request → spans ingested correctly
  - [x] OTLP service name extracted from resource attributes
  - [x] Large batch (1000 spans) → all accepted or correct subset sampled

---

## Phase 9 — Storage

- [x] Define `TraceStore` interface: `Upsert`, `Get`, `Query`, `Services`, `Operations`, `Stats`
- [x] Implement `MemoryStore`:
  - [x] Primary map `map[TraceID]*Trace` under RWMutex
  - [x] `byService`: `map[string]map[TraceID]struct{}`
  - [x] `byOperation`: `map[string]map[TraceID]struct{}` (key = `service:operation`)
  - [x] `byError`: `map[TraceID]struct{}`
  - [x] `timeline []timelineEntry` for eviction ordering
  - [x] `Upsert`: add to all indexes, evict if over `maxSize`
  - [x] `evict()`: remove oldest entries from all indexes
- [x] Implement `Query(q *TraceQuery) (*TraceQueryResult, error)`:
  - [x] Start with all traceIDs from `byService[q.ServiceName]` (or all if empty)
  - [x] Intersect with `byOperation` filter
  - [x] Intersect with `byError` filter
  - [x] For each candidate: load trace, apply duration/time range/tag filters
  - [x] Sort, paginate, return summaries
- [x] Write storage tests:
  - [x] Upsert + Get roundtrip
  - [x] Query by service returns only matching traces
  - [x] Query by operation (service:op) filters correctly
  - [x] Query by error status returns only error traces
  - [x] Duration filter: min and max bounds correct
  - [x] Pagination: offset + limit work correctly
  - [x] Sort by duration descending: highest first
  - [x] Eviction: after maxSize exceeded, oldest traces removed
  - [x] Concurrent Upsert + Query with `-race`

---

## Phase 10 — Analysis

- [x] Implement `ComputeCriticalPath(trace *Trace) []*Span`:
  - [x] DFS from root, track `longestPath(span) (path, duration)` at each node
  - [x] For spans with multiple children: take child with maximum `longestPath` duration
  - [x] Return ordered slice from root to leaf
- [x] Implement `DetectParallelGroups(span *Span) []ParallelGroup`:
  - [x] For each parent span: sort children by startTime
  - [x] Sweep line: children overlap if child[i].StartTime < child[i-1].EndTime
  - [x] Group overlapping children into ParallelGroups
- [x] Implement `DetectGaps(trace *Trace) []SpanGap`:
  - [x] For each parent-child pair: gap = time between parent's previous child end and next child start
  - [x] Gap between parent start and first child start
  - [x] Gap between last child end and parent end
- [x] Implement `BuildDependencyGraph(traces []*Trace, since time.Time) *DependencyGraph`:
  - [x] For each CLIENT span in recent traces: extract caller/callee pair
  - [x] Callee = `span.Attributes["peer.service"]` or derive from operation name
  - [x] Accumulate count, error count, durations per edge
  - [x] Compute P99 from duration slice (sort + percentile)
- [x] Implement `CompareTraces(base, compare *Trace) *TraceComparison`:
  - [x] Match spans by `(serviceName, operationName)` pair + positional order
  - [x] For matched pairs: compute duration delta, attribute diffs
  - [x] Identify spans only in base or only in compare
- [x] Write analysis tests:
  - [x] Single span: critical path = [span]
  - [x] Linear chain A→B→C: critical path = [A, B, C]
  - [x] Parallel children: critical path takes longest branch only
  - [x] ParallelGroups: overlapping siblings detected, non-overlapping not grouped
  - [x] Gaps: missing time between sequential siblings detected
  - [x] DependencyGraph: 10 traces → correct edge counts and error rates
  - [x] Compare: same trace = empty diff; added span = appears in onlyInCompare

---

## Phase 11 — RED Metrics

- [x] Implement `SlidingWindow` (60×1s ring buffer): `Add(n int64)`, `Rate() float64`
- [x] Implement `Histogram` (reservoir sampling, 1024 slots):
  - [x] `Record(ms float64)`: reservoir sampling algorithm
  - [x] `Percentile(p float64) float64`: sort reservoir, index by p
  - [x] `P50()`, `P95()`, `P99()`, `P999()` convenience methods
- [x] Implement `MetricsStore`:
  - [x] `map[MetricKey]*ServiceMetrics` under RWMutex
  - [x] `Record(span *Span)`: update rate, error rate, histogram for `(service, operation)`
  - [x] `Snapshot() []MetricSnapshot`: current values for all keys
  - [x] `HeatmapBuckets(service, op string, window, resolution time.Duration) []HeatmapBucket`
- [x] Write metrics tests:
  - [x] Record 100 spans for same key → rate, error rate correct
  - [x] P99 with known distribution (100 values 1..100ms) → P99 ≈ 99ms ± 2ms
  - [x] Sliding window rate: record 10 spans/s for 5s → Rate() ≈ 10
  - [x] Concurrent Record from 50 goroutines → `-race` clean

---

## Phase 12 — Demo Microservices SDK

- [x] Implement `DemoSDK`:
  - [x] `NewSpan(ctx, name, kind) (context.Context, *Span)`: create span, attach to context
  - [x] `FinishSpan(span, opts...)`: set EndTime, add final attributes
  - [x] `InjectHTTP(ctx, headers)`: propagate active span via W3C propagator
  - [x] `ExtractHTTP(req) context.Context`: extract parent context from request
  - [x] `Export(spans []*Span)`: POST to collector URL
  - [x] `AddEvent(span, name, attrs)`: add timestamped event
  - [x] `SetError(span, err)`: set StatusError + record error message as event
- [x] Implement 4 demo services as goroutines (simulate HTTP calls with real latency via `time.Sleep(jitter(min, max))`):
  - [x] `runFrontendSvc`: creates root span, calls api-gateway
  - [x] `runAPIGateway`: creates server span, starts inventory + payment calls in goroutines (parallel)
  - [x] `runInventorySvc`: creates server span, creates "db.query" child span (5-15ms) and "cache.get" child span (1-3ms, 80% hit)
  - [x] `runPaymentSvc`: creates server span, creates "redis.get" (2-5ms), "stripe.charge" (50-200ms, 5% fail rate), "db.insert" (5-10ms) children
- [x] Implement scenario functions (see SPEC.md scenario list)
- [x] Implement `Runner.Start()`: background goroutine that picks weighted random scenario and runs it every 500ms–3s
- [x] Write demo SDK tests:
  - [x] `NewSpan` + `FinishSpan`: span has valid IDs, correct times
  - [x] Nested spans: child has parent's SpanID as ParentSpanID
  - [x] `InjectHTTP` + `ExtractHTTP` roundtrip: parent context preserved
  - [x] `Export`: spans reach mock collector server
  - [x] Scenario `successful_checkout`: produces correct number of spans with correct structure

---

## Phase 13 — HTTP API + SSE

- [x] Set up chi router with CORS (allow localhost:5173), logging, recovery middleware
- [x] Implement SSE bus: `chan SSEEvent`, fan-out to per-connection channels (buffered 100)
- [x] Implement all ingest handlers (Phase 8)
- [x] Implement query handlers:
  - [x] `GET /api/v1/traces` with all query params
  - [x] `GET /api/v1/traces/:traceId` with full span list + analysis
  - [x] `GET /api/v1/services`
  - [x] `GET /api/v1/operations?service=X`
  - [x] `GET /api/v1/dependencies`
  - [x] `GET /api/v1/traces/compare?base=X&compare=Y`
- [x] Implement metrics handlers:
  - [x] `GET /api/v1/metrics/red`
  - [x] `GET /api/v1/metrics/heatmap`
- [x] Implement sampler handlers:
  - [x] `GET /api/v1/sampler`
  - [x] `PUT /api/v1/sampler` — swap sampler type live, validate config
- [x] Implement SSE handlers:
  - [x] `GET /sse/spans`
  - [x] `GET /sse/traces`
  - [x] `GET /sse/metrics`
  - [x] `GET /sse/sampler`
  - [x] All SSE handlers: set proper headers, detect client disconnect via `r.Context().Done()`
- [x] Write API tests with `httptest`:
  - [x] Ingest → query returns same trace
  - [x] Filter by service returns only matching traces
  - [x] Dependencies endpoint after 50 traces → non-empty graph
  - [x] Sampler PUT changes type → GET confirms new type
  - [x] 404 for unknown trace ID
  - [x] 400 for malformed span input

---

## Phase 14 — Frontend Infrastructure

- [x] React Router v6: routes `/` (Search), `/trace/:id`, `/map`, `/metrics`, `/sampler`, `/compare`
- [x] TypeScript types mirroring all backend DTOs exactly
- [x] API client: typed functions for every endpoint
- [x] `useSSE(url, onEvent)` hook: reconnect on error with 2s delay
- [x] Zustand store: `activeSampler`, `services`, `metrics`, `liveSpans` (ring buffer 500)
- [x] Service color palette: 10 distinct colors, `getServiceColor(name: string): string`
- [x] Nav component with active link styling

---

## Phase 15 — Search Page

- [x] FilterBar: service select (from `/api/v1/services`), operation select, status toggle, duration sliders, time range select
- [x] `useSearch` hook: debounce filter changes 300ms, call API
- [x] TraceCard: service dots row, root op text, duration bar, span count, time ago badge
- [x] Duration bar: width proportional to duration relative to max in current result set
- [x] Live: SSE `sse/traces` → prepend new card with Framer Motion `AnimatePresence` slide-down
- [x] Pagination: load more button
- [x] Empty state: "No traces found. Start the demo to generate traces."

---

## Phase 16 — Waterfall Chart (D3) — Most Important Component

- [x] `WaterfallChart` React component accepts `trace: TraceDetail` prop
- [x] Build `spanRows` array: depth-first traversal of span tree, producing ordered array with `depth`, `startMs` (offset from trace start), `durationMs`
- [x] Create D3 scales:
  - [x] `xScale = d3.scaleLinear().domain([0, trace.durationMs]).range([labelWidth, svgWidth])`
  - [x] Row height = 24px, label width = 220px
- [x] Render SVG:
  - [x] TimeAxis: `d3.axisTop(xScale)` with adaptive ticks
  - [x] Per row: service label (colored, truncated), span bar rect
  - [x] Critical path rows: add amber 2px border rect overlay
  - [x] Error rows: `fill="#dc2626"` + CSS `animation: pulse 2s infinite`
  - [x] Span kind pill: `<text>` label inside bar if bar is wide enough
- [x] `d3.zoom()` on SVG container: rescale xScale, re-render axis and bars
- [x] Minimap: 120px-tall fixed `<svg>` at bottom, shows all spans in overview, drag to pan main view
- [x] Hover: `<title>` or custom tooltip div with service/op/duration/start
- [x] Click row: call `onSpanSelect(span)` to open SpanDrawer
- [x] Write waterfall tests:
  - [x] With 1-span trace: renders 1 row
  - [x] With 14-span trace: renders 14 rows in correct order
  - [x] Critical path spans have amber border class
  - [x] Error spans have red fill class

---

## Phase 17 — Span Drawer

- [x] shadcn `Sheet` from right side, 480px wide
- [x] Header: operation name, service badge
- [x] Stats row: duration, start offset, kind badge, status badge
- [x] Parent link: if has parent, show clickable "(parent: op name)" → scroll waterfall to parent
- [x] Attributes table: key / value / type badge columns
- [x] Events section: if events present, timeline list with timestamp offset + name
- [x] Links section: cross-trace links with click-to-navigate

---

## Phase 18 — Service Map

- [x] Fetch `/api/v1/dependencies` on load + every 30s
- [x] Convert to React Flow nodes/edges
- [x] `ServiceNode` custom node: circle, radius=`20 + log(spanCount)*5`, color from error rate HSL formula
- [x] `DependencyEdge` custom edge: stroke-width from `1 + log(count)`, label = P99 ms
- [x] dagre layout: `rankdir: 'LR'`, `nodesep: 80`, `ranksep: 120`
- [x] Side panel: click node → show top operations table + sparklines
- [x] Animate: high-traffic edges get moving dots (Framer Motion on edge path)
- [x] Empty state: "No service dependencies yet. Waiting for traces..."

---

## Phase 19 — Metrics Page

- [x] Service selector dropdown (from `/api/v1/services`)
- [x] Fetch RED metrics on service change
- [x] SSE `/sse/metrics` → update charts live
- [x] Three Recharts charts per service: Rate (LineChart), ErrorRate (AreaChart red), Latency P50/P95/P99 (LineChart multi-line)
- [x] Operations table: sortable columns (op name, rate, error%, P50, P95, P99)
- [x] Heatmap chart: fetch `/api/v1/metrics/heatmap`, render as stacked bar (spans=blue, errors=red)

---

## Phase 20 — Sampler Config Page

- [x] Fetch current sampler from `/api/v1/sampler`
- [x] SSE `/sse/sampler` → update live stats card
- [x] Stats card: sampling rate gauge, observed throughput, total sampled, total dropped
- [x] Throughput chart: Recharts LineChart, sampling rate % over time (60-point ring buffer)
- [x] Sampler type radio buttons: Always / Never / Probabilistic / RateLimit / Adaptive / Rules / Tail
- [x] Config form per type:
  - [x] Probabilistic: rate slider 0-100%
  - [x] RateLimit: traces/sec number input
  - [x] Adaptive: target rate input, min/max rate sliders
  - [x] Rules: table of rules with add/remove + priority number
  - [x] Tail: buffer timeout + policy chain builder
- [x] Apply button: PUT → show diff confirmation dialog → apply

---

## Phase 21 — Compare Page

- [x] URL `/compare?base=:id&compare=:id`
- [x] Fetch both traces
- [x] Fetch comparison from `/api/v1/traces/compare`
- [x] Two WaterfallChart components side by side
- [x] Synchronize time axis: use `Math.max(base.durationMs, compare.durationMs)` for both
- [x] Overlay diff coloring: matched pairs show duration delta badge; only-in-base grayed; only-in-compare green-highlighted
- [x] Summary diff row: total duration delta, span count delta, error delta

---

## Phase 22 — Integration and Polish

- [x] Smoke test: start collector + demo → within 10s, Search page shows traces
- [x] Smoke test: click any trace → waterfall renders with correct span count
- [x] Smoke test: payment_timeout scenario → red error spans visible in waterfall
- [x] Smoke test: Service Map shows api-gateway → payment-svc edge
- [x] Smoke test: PUT sampler to probabilistic 0.1 → sampler stats show ~10% rate
- [x] Smoke test: Compare two traces of same scenario → diff shows correctly
- [x] `go test ./... -race -count=1` — zero failures
- [x] `npx tsc --noEmit` — zero errors
- [x] README: 3-command setup, screenshot description, architecture overview

---

## Testing Requirements Summary

| Component | Critical tests |
|---|---|
| TraceID/SpanID | Parse roundtrip, invalid rejection, uniqueness |
| Probabilistic sampler | Cross-service consistency (CRITICAL), rate accuracy |
| Tail sampler | Error policy, latency policy, timeout, maxSize eviction |
| W3C propagation | Inject+extract roundtrip, malformed input |
| Assembler | Out-of-order spans, orphans, parallel children |
| MemoryStore | Eviction, concurrent access, all query filter combinations |
| Critical path | Linear, parallel, single span |
| RED metrics | P99 accuracy, concurrent Record |
| API | All endpoints, error cases |
| Demo SDK | Span structure, context propagation |
