# Distributed Tracing System (Jaeger-lite) — Technical Specification

## Overview

A production-quality distributed tracing system built from scratch. The backend is Go and implements span collection via HTTP/JSON and OTLP, six sampling strategies including tail-based and adaptive sampling, in-memory trace storage with indexed search, critical-path analysis, service dependency graph computation, and RED metrics. The frontend is React + TypeScript with a D3.js waterfall trace visualizer, React Flow service map, and live SSE-powered dashboards. Four demo microservices ship with the system and continuously generate realistic traces.

---

## Tech Stack

### Backend
| Concern | Choice |
|---|---|
| Language | Go 1.22+ |
| HTTP Router | `chi` v5 |
| Live Events | Server-Sent Events (stdlib) |
| ID Generation | `crypto/rand` (128-bit TraceID, 64-bit SpanID) |
| Testing | `testing` + `testify` + `go test -race` |

### Frontend
| Concern | Choice |
|---|---|
| Language | TypeScript 5+ strict |
| Framework | React 18 + Vite 5 |
| Styling | Tailwind CSS v3 + shadcn/ui |
| Waterfall Chart | **D3.js v7** (custom – not recharts, critical for this UI) |
| Service Map | `@xyflow/react` + `@dagrejs/dagre` |
| Other Charts | Recharts (metrics) |
| Animation | Framer Motion |
| State | Zustand |
| Live Data | native `EventSource` (SSE) |
| HTTP | `ky` |

---

## Project Structure

```
tracing/
├── cmd/
│   ├── collector/main.go        # Span collector server (port 4318)
│   └── demo/main.go             # Demo microservices runner
├── internal/
│   ├── model/
│   │   ├── ids.go               # TraceID [16]byte, SpanID [8]byte
│   │   ├── span.go              # Span, SpanKind, SpanStatus, SpanEvent, SpanLink
│   │   ├── trace.go             # Trace (assembled), TraceTree, ParallelGroup, SpanGap
│   │   └── keyvalue.go          # KeyValue (OTEL attribute), AnyValue union
│   ├── collector/
│   │   ├── http.go              # POST /api/v1/spans (native JSON)
│   │   ├── otlp.go              # POST /v1/traces (OTLP HTTP/JSON)
│   │   └── pipeline.go          # validate → sample → enrich → assemble → store → broadcast
│   ├── propagation/
│   │   ├── propagator.go        # Propagator interface + SpanContext carrier
│   │   ├── w3c.go               # W3C TraceContext (traceparent + tracestate)
│   │   ├── b3.go                # B3 multi-header + B3 single-header
│   │   └── composite.go         # Try W3C first, then B3
│   ├── sampler/
│   │   ├── sampler.go           # Sampler interface + SamplingResult
│   │   ├── always.go            # AlwaysSample / NeverSample
│   │   ├── probabilistic.go     # TraceID-hash (consistent across services)
│   │   ├── ratelimit.go         # Token bucket N traces/sec
│   │   ├── parent.go            # ParentBased (respect upstream decision)
│   │   ├── adaptive.go          # PID-like auto rate adjustment
│   │   ├── rules.go             # Match by service/operation/tags/status
│   │   └── tail.go              # Buffer + policy evaluation at completion
│   ├── storage/
│   │   ├── store.go             # TraceStore interface
│   │   ├── memory.go            # In-memory with multi-field indexes + eviction
│   │   └── query.go             # Query builder + filter execution
│   ├── processor/
│   │   ├── assembler.go         # Group spans by TraceID, build parent-child tree
│   │   └── enricher.go          # Add depth, hasError, serviceName from resource
│   ├── analysis/
│   │   ├── critical_path.go     # DFS longest-path computation
│   │   ├── dependency.go        # Service dependency graph builder
│   │   └── compare.go           # Trace diff (align spans, compute deltas)
│   ├── metrics/
│   │   ├── red.go               # Per (service, operation): Rate, Errors, Duration
│   │   ├── histogram.go         # Reservoir-sampling P50/P95/P99
│   │   └── window.go            # 60×1s sliding window counters
│   └── demo/
│       ├── sdk.go               # Minimal tracing SDK (NewTrace/NewSpan/Finish/Inject/Extract)
│       ├── services.go          # 4 demo service implementations (goroutines)
│       ├── scenarios.go         # Weighted scenario generators
│       └── runner.go            # Background loop: pick scenario → run → sleep → repeat
├── api/
│   ├── server.go
│   ├── handler_ingest.go        # POST /api/v1/spans, POST /v1/traces
│   ├── handler_query.go         # GET traces/search/services/operations/compare
│   ├── handler_analysis.go      # GET dependencies, critical-path
│   ├── handler_metrics.go       # GET RED metrics, heatmap
│   ├── handler_sampler.go       # GET/PUT sampler config
│   ├── sse.go                   # SSE broadcaster
│   ├── middleware.go
│   └── dto.go
├── web/
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Search.tsx
│   │   │   ├── TraceDetail.tsx
│   │   │   ├── ServiceMap.tsx
│   │   │   ├── Metrics.tsx
│   │   │   ├── Sampler.tsx
│   │   │   └── Compare.tsx
│   │   ├── components/
│   │   │   ├── waterfall/
│   │   │   │   ├── WaterfallChart.tsx   # D3 container + zoom/pan
│   │   │   │   ├── SpanRow.tsx
│   │   │   │   ├── TimeAxis.tsx
│   │   │   │   └── SpanDrawer.tsx       # Slide-in detail panel
│   │   │   ├── servicemap/
│   │   │   │   ├── ServiceNode.tsx
│   │   │   │   └── DependencyEdge.tsx
│   │   │   ├── search/
│   │   │   │   ├── TraceCard.tsx
│   │   │   │   └── FilterBar.tsx
│   │   │   └── shared/
│   │   │       ├── SpanKindBadge.tsx
│   │   │       ├── StatusBadge.tsx
│   │   │       └── ServiceTag.tsx
│   │   ├── store/tracingStore.ts
│   │   ├── hooks/
│   │   │   ├── useSSE.ts
│   │   │   └── useSearch.ts
│   │   ├── api/client.ts
│   │   └── types/index.ts
│   └── package.json
├── go.mod
└── Makefile
```

---

## Core Data Model

### IDs

```go
// internal/model/ids.go
type TraceID [16]byte   // 128-bit, OpenTelemetry-compatible
type SpanID  [8]byte    // 64-bit

func (t TraceID) String() string { return hex.EncodeToString(t[:]) }  // 32 hex chars
func (s SpanID)  String() string { return hex.EncodeToString(s[:]) }  // 16 hex chars
func (t TraceID) IsZero() bool   { return t == TraceID{} }
func (s SpanID)  IsZero() bool   { return s == SpanID{} }

func NewTraceID() (TraceID, error)  // crypto/rand
func NewSpanID()  (SpanID, error)   // crypto/rand
func ParseTraceID(s string) (TraceID, error)  // validate 32 hex chars
func ParseSpanID(s string)  (SpanID, error)   // validate 16 hex chars

var ZeroTraceID TraceID
var ZeroSpanID  SpanID
```

### Span

```go
// internal/model/span.go
type SpanKind   int
type StatusCode int

const (
    SpanKindInternal SpanKind = 1
    SpanKindServer   SpanKind = 2
    SpanKindClient   SpanKind = 3
    SpanKindProducer SpanKind = 4
    SpanKindConsumer SpanKind = 5
)

const (
    StatusUnset StatusCode = 0
    StatusOK    StatusCode = 1
    StatusError StatusCode = 2
)

type SpanStatus struct {
    Code    StatusCode
    Message string
}

type SpanEvent struct {
    Time       time.Time
    Name       string
    Attributes []KeyValue
}

type SpanLink struct {
    TraceID    TraceID
    SpanID     SpanID
    TraceState string
    Attributes []KeyValue
}

type Span struct {
    TraceID      TraceID
    SpanID       SpanID
    ParentSpanID SpanID      // zero value = root span
    TraceState   string

    Name         string
    Kind         SpanKind
    ServiceName  string
    ServiceAttrs map[string]string

    StartTime time.Time     // nanosecond precision
    EndTime   time.Time

    Attributes []KeyValue
    Events     []SpanEvent
    Links      []SpanLink
    Status     SpanStatus

    // Computed by assembler
    Depth    int
    HasError bool      // true if StatusError OR any descendant has error
    Children []*Span   // populated during tree construction

    ReceivedAt time.Time
}

func (s *Span) Duration() time.Duration { return s.EndTime.Sub(s.StartTime) }
func (s *Span) IsRoot() bool            { return s.ParentSpanID.IsZero() }
```

### Trace

```go
// internal/model/trace.go
type Trace struct {
    TraceID  TraceID
    Spans    []*Span   // all spans, including orphans
    RootSpan *Span

    // Computed by assembler
    Services    []string       // unique service names sorted
    Duration    time.Duration  // RootSpan.Duration() or max span end - min span start
    SpanCount   int
    ErrorCount  int

    // Computed by analysis
    CriticalPath  []*Span
    ParallelGroups []ParallelGroup
    Gaps           []SpanGap

    ReceivedAt  time.Time
    CompletedAt time.Time
}

type ParallelGroup struct {
    Spans     []*Span
    StartTime time.Time
    EndTime   time.Time
}

type SpanGap struct {
    Before   *Span
    After    *Span
    Duration time.Duration
}
```

### KeyValue

```go
// internal/model/keyvalue.go
type ValueType int

const (
    ValueString ValueType = iota
    ValueInt
    ValueFloat
    ValueBool
    ValueStringSlice
)

type KeyValue struct {
    Key   string
    Type  ValueType
    SVal  string
    IVal  int64
    FVal  float64
    BVal  bool
    SArr  []string
}

func StringKV(k, v string) KeyValue
func IntKV(k string, v int64) KeyValue
func BoolKV(k string, v bool) KeyValue
func FloatKV(k string, v float64) KeyValue
```

---

## Sampling System

### Interface

```go
// internal/sampler/sampler.go
type SamplingDecision int

const (
    Drop      SamplingDecision = iota
    RecordOnly                  // record locally, don't export
    Sample                      // record and export
)

type SamplingParameters struct {
    TraceID       TraceID
    SpanID        SpanID
    ParentSpanID  SpanID
    OperationName string
    ServiceName   string
    Kind          SpanKind
    Attributes    []KeyValue
    ParentSampled *bool   // nil = no parent info
}

type SamplingResult struct {
    Decision   SamplingDecision
    Attributes []KeyValue   // e.g., sampler.type=probabilistic
    Reason     string
}

type Sampler interface {
    ShouldSample(p SamplingParameters) SamplingResult
    Name() string
    Config() map[string]any
}
```

### Probabilistic Sampler

CRITICAL: must use TraceID as input so ALL services in a distributed system make the SAME decision for the same trace. Never use random number alone.

```go
// internal/sampler/probabilistic.go
type ProbabilisticSampler struct {
    mu        sync.RWMutex
    rate      float64   // 0.0–1.0
    threshold uint64    // uint64(rate * math.MaxUint64)
}

func (s *ProbabilisticSampler) ShouldSample(p SamplingParameters) SamplingResult {
    id := binary.BigEndian.Uint64(p.TraceID[:8])
    s.mu.RLock()
    threshold := s.threshold
    s.mu.RUnlock()
    if id < threshold {
        return SamplingResult{Decision: Sample, Reason: fmt.Sprintf("probabilistic rate=%.4f", s.rate)}
    }
    return SamplingResult{Decision: Drop}
}

func (s *ProbabilisticSampler) SetRate(rate float64) {
    s.mu.Lock()
    s.rate = math.Max(0, math.Min(1, rate))
    s.threshold = uint64(s.rate * float64(math.MaxUint64))
    s.mu.Unlock()
}
```

### Rate-Limiting Sampler (Token Bucket)

```go
// internal/sampler/ratelimit.go
type RateLimitSampler struct {
    mu           sync.Mutex
    tracesPerSec float64
    tokens       float64   // current tokens
    maxTokens    float64   // burst = tracesPerSec * 2
    lastRefill   time.Time
}

func (s *RateLimitSampler) ShouldSample(p SamplingParameters) SamplingResult {
    s.mu.Lock()
    defer s.mu.Unlock()
    now := time.Now()
    elapsed := now.Sub(s.lastRefill).Seconds()
    s.tokens = math.Min(s.maxTokens, s.tokens + elapsed*s.tracesPerSec)
    s.lastRefill = now
    if s.tokens >= 1.0 {
        s.tokens--
        return SamplingResult{Decision: Sample, Reason: "rate-limit granted"}
    }
    return SamplingResult{Decision: Drop, Reason: "rate-limit exceeded"}
}
```

### Adaptive Sampler

```go
// internal/sampler/adaptive.go
// Automatically adjusts sampling probability to maintain target throughput.
// Uses multiplicative adjustment: newRate = currentRate * (target / observed)
// Applies hysteresis: only adjusts if deviation > 10% for 3 consecutive windows.

type AdaptiveSampler struct {
    targetRate   float64   // desired traces/second
    minRate      float64   // floor: 0.001
    maxRate      float64   // ceiling: 1.0
    adjustPeriod time.Duration
    inner        *ProbabilisticSampler
    window       *SlidingWindow   // track observed throughput
    mu           sync.Mutex
    deviations   int  // consecutive windows with >10% deviation
}

// Adjust() runs every adjustPeriod:
//   observed := window.Rate()
//   if observed == 0: return (no data yet)
//   ratio := targetRate / observed
//   if math.Abs(ratio-1.0) < 0.10: deviations = 0; return  (within 10%)
//   deviations++
//   if deviations < 3: return  (hysteresis: wait 3 windows)
//   newRate := inner.rate * ratio
//   newRate = clamp(newRate, minRate, maxRate)
//   inner.SetRate(newRate)
//   deviations = 0
```

### Rule-Based Sampler

```go
// internal/sampler/rules.go
type Rule struct {
    ServiceName   string            // "" = any
    OperationGlob string            // glob pattern, "" = any
    MinDuration   time.Duration
    StatusCode    *StatusCode       // nil = any
    Tags          map[string]string // all must match
    Decision      SamplingDecision
    Priority      int               // higher evaluated first
}

type RuleBasedSampler struct {
    rules    []Rule    // sorted by priority desc
    fallback Sampler
}
```

### Tail-Based Sampler

```go
// internal/sampler/tail.go
// Buffer spans until trace is "complete" (timeout), then apply policies.
// Policy chain: first matching policy wins.

type TailPolicy interface {
    Name() string
    Evaluate(spans []*Span, duration time.Duration) (keep bool, reason string)
}

// ErrorPolicy:       keep if any span has StatusError
// LatencyPolicy:     keep if duration >= threshold
// ProbabilisticPolicy: keep with probability P (traceID-based for consistency)

type TailSampler struct {
    mu       sync.Mutex
    buffer   map[TraceID]*tailBuffer
    timeout  time.Duration   // default 10s
    maxSize  int             // max buffered traces (default 10000)
    policies []TailPolicy
    accept   func([]*Span)   // callback for accepted traces
    reject   func(TraceID)   // callback for rejected traces
    stop     chan struct{}
}

type tailBuffer struct {
    spans   []*Span
    firstAt time.Time
    timer   *time.Timer
}

// AddSpan: if trace already decided: drop or accept immediately
//          if new trace: create buffer entry + timer
// OnTimer: evaluate policies, call accept or reject callback
```

### Parent-Based Sampler

```go
// internal/sampler/parent.go
// For spans with a parent: use parent's sampling decision.
// For root spans: use the root sampler.
// This ensures all spans in a distributed trace are sampled/dropped together.

type ParentBasedSampler struct {
    root                  Sampler  // for new traces
    remoteParentSampled   Sampler  // parent was sampled (usually AlwaysSample)
    remoteParentNotSampled Sampler // parent was not sampled (usually NeverSample)
}

func (s *ParentBasedSampler) ShouldSample(p SamplingParameters) SamplingResult {
    if p.ParentSpanID.IsZero() {
        return s.root.ShouldSample(p)
    }
    if p.ParentSampled != nil {
        if *p.ParentSampled {
            return s.remoteParentSampled.ShouldSample(p)
        }
        return s.remoteParentNotSampled.ShouldSample(p)
    }
    return s.root.ShouldSample(p)
}
```

---

## Context Propagation

### W3C TraceContext

```go
// internal/propagation/w3c.go
// traceparent: 00-{32hex traceID}-{16hex spanID}-{02hex flags}
// flags byte: bit 0 = sampled

type W3CPropagator struct{}

func (W3CPropagator) Inject(ctx SpanContext, headers http.Header) {
    flags := "00"
    if ctx.IsSampled { flags = "01" }
    headers.Set("traceparent", fmt.Sprintf("00-%s-%s-%s",
        ctx.TraceID, ctx.SpanID, flags))
    if ctx.TraceState != "" {
        headers.Set("tracestate", ctx.TraceState)
    }
}

func (W3CPropagator) Extract(headers http.Header) (SpanContext, bool) {
    val := headers.Get("traceparent")
    if val == "" { return SpanContext{}, false }
    parts := strings.SplitN(val, "-", 4)
    if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
        return SpanContext{}, false
    }
    traceID, err1 := ParseTraceID(parts[1])
    spanID, err2  := ParseSpanID(parts[2])
    if err1 != nil || err2 != nil { return SpanContext{}, false }
    sampled := len(parts[3]) >= 2 && parts[3][len(parts[3])-1]&1 != 0
    return SpanContext{
        TraceID: traceID, SpanID: spanID, IsSampled: sampled,
        TraceState: headers.Get("tracestate"), IsRemote: true,
    }, true
}
```

### B3 Multi-Header

```go
// internal/propagation/b3.go
// X-B3-TraceId:     {32hex}
// X-B3-SpanId:      {16hex}
// X-B3-ParentSpanId:{16hex} (optional)
// X-B3-Sampled:     1 | 0

type B3Propagator struct{ SingleHeader bool }
```

### SpanContext

```go
// internal/propagation/propagator.go
type SpanContext struct {
    TraceID    TraceID
    SpanID     SpanID
    TraceState string
    IsSampled  bool
    IsRemote   bool
}

func (s SpanContext) IsValid() bool {
    return !s.TraceID.IsZero() && !s.SpanID.IsZero()
}

type Propagator interface {
    Inject(ctx SpanContext, headers http.Header)
    Extract(headers http.Header) (SpanContext, bool)
}
```

---

## Storage

### In-Memory Store

```go
// internal/storage/memory.go
type MemoryStore struct {
    mu    sync.RWMutex
    traces map[TraceID]*Trace

    // Inverted indexes
    byService   map[string]map[TraceID]struct{}
    byOperation map[string]map[TraceID]struct{}  // "service:operation" → set
    byError     map[TraceID]struct{}

    // Ordered timeline for eviction (ring buffer)
    timeline []timelineEntry  // sorted by ReceivedAt ascending
    maxSize  int
    maxAge   time.Duration

    broadcast chan<- SSEEvent
    metrics   *metrics.Store
}

type timelineEntry struct {
    id         TraceID
    receivedAt time.Time
}

func (s *MemoryStore) Upsert(trace *Trace) error
func (s *MemoryStore) Get(id TraceID) (*Trace, bool)
func (s *MemoryStore) Query(q *TraceQuery) (*TraceQueryResult, error)
func (s *MemoryStore) Services() []string
func (s *MemoryStore) Operations(service string) []string
func (s *MemoryStore) Stats() StoreStats
func (s *MemoryStore) evictOld()   // called on every Upsert if over limit
```

### Query

```go
// internal/storage/query.go
type TraceQuery struct {
    ServiceName   string
    OperationName string
    Tags          map[string]string
    MinDuration   *time.Duration
    MaxDuration   *time.Duration
    HasError      *bool
    StartTime     *time.Time
    EndTime       *time.Time
    TraceID       *TraceID
    Limit         int
    Offset        int
    SortBy        string   // "duration" | "receivedAt" | "spanCount"
    SortDesc      bool
}

type TraceQueryResult struct {
    Traces  []*TraceSummary
    Total   int
    HasMore bool
}

type TraceSummary struct {
    TraceID     TraceID
    RootService string
    RootOp      string
    Duration    time.Duration
    SpanCount   int
    Services    []string
    HasError    bool
    ReceivedAt  time.Time
}
```

---

## Critical Path Algorithm

```go
// internal/analysis/critical_path.go
// The critical path is the sequence of causally-linked spans with maximum total duration.
// For a span with parallel children, only the longest child branch is on the critical path.

func ComputeCriticalPath(trace *Trace) []*Span {
    if trace.RootSpan == nil { return nil }
    path, _ := longestPath(trace.RootSpan)
    return path
}

func longestPath(span *Span) (path []*Span, duration time.Duration) {
    if len(span.Children) == 0 {
        return []*Span{span}, span.Duration()
    }
    var bestChildPath []*Span
    var bestChildDur  time.Duration
    for _, child := range span.Children {
        childPath, childDur := longestPath(child)
        if childDur > bestChildDur {
            bestChildDur  = childDur
            bestChildPath = childPath
        }
    }
    return append([]*Span{span}, bestChildPath...), span.Duration() + bestChildDur
}

// ParallelGroup detection:
// For each parent, group children whose time windows overlap.
// Children [A, B, C] with A=[10,50], B=[12,45], C=[55,90]:
//   Group1 = {A, B} (overlap), Group2 = {C} (no overlap with group1)
```

---

## Service Dependency Graph

```go
// internal/analysis/dependency.go
type ServiceEdge struct {
    Caller       string
    Callee       string
    Count        int64
    ErrorCount   int64
    P99Duration  time.Duration
    durations    []float64   // for P99 computation
}

type DependencyGraph struct {
    Services    []ServiceNode
    Edges       []*ServiceEdge
}

type ServiceNode struct {
    Name        string
    SpanCount   int64
    ErrorRate   float64
    P99Ms       float64
    ReqPerSec   float64
}

// Build edges from CLIENT spans:
// For each span where Kind == SpanKindClient:
//   caller = span.ServiceName
//   callee = span.Attributes["peer.service"] or span.Attributes["db.system"] etc.
//   AddEdge(caller, callee, span.Duration(), span.HasError)
```

---

## RED Metrics

```go
// internal/metrics/red.go
type MetricKey struct {
    Service   string
    Operation string
}

type ServiceMetrics struct {
    Key       MetricKey
    Rate      *SlidingWindow    // spans arriving per second
    Errors    *SlidingWindow    // error spans per second
    Durations *Histogram        // reservoir sample for P50/P95/P99
}

// Reservoir sampling histogram (size 1024)
type Histogram struct {
    mu      sync.Mutex
    samples []float64
    n       int64      // total observations
    cap     int        // reservoir size
}

func (h *Histogram) Record(ms float64) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.n++
    if int(h.n) <= h.cap {
        h.samples = append(h.samples, ms)
    } else {
        // Reservoir sampling: replace with probability cap/n
        j := rand.Int63n(h.n)
        if j < int64(h.cap) {
            h.samples[j] = ms
        }
    }
}

func (h *Histogram) Percentile(p float64) float64 {
    h.mu.Lock()
    sorted := append([]float64{}, h.samples...)
    h.mu.Unlock()
    sort.Float64s(sorted)
    if len(sorted) == 0 { return 0 }
    idx := int(p * float64(len(sorted)-1))
    return sorted[idx]
}
```

---

## Traffic Generator

### Service Call Graph

```
frontend-svc  → api-gateway   (HTTP GET /api/checkout)
api-gateway   → inventory-svc (HTTP GET /api/inventory) ─┐ parallel
api-gateway   → payment-svc   (HTTP POST /api/charge)   ─┘
inventory-svc → [db.query span, cache.get span]  (simulated)
payment-svc   → [redis.get span, stripe.charge span, db.insert span] (simulated)
```

### Trace Generator SDK

```go
// internal/demo/sdk.go
// Minimal SDK for simulated services to create and propagate spans.

type DemoSDK struct {
    serviceName string
    collector   string  // collector URL: http://localhost:4318
    sampler     Sampler
    propagator  Propagator
    httpClient  *http.Client
}

func (s *DemoSDK) StartSpan(ctx context.Context, name string, kind SpanKind, opts ...SpanOption) (context.Context, *Span)
func (s *DemoSDK) FinishSpan(span *Span, opts ...FinishOption)
func (s *DemoSDK) InjectHTTP(ctx context.Context, headers http.Header)
func (s *DemoSDK) ExtractHTTP(req *http.Request) context.Context
func (s *DemoSDK) Export(spans []*Span) error   // POST to collector
```

### Scenarios

```go
// internal/demo/scenarios.go
var Scenarios = []Scenario{
    {Name: "successful_checkout",  Weight: 60, Run: runSuccessfulCheckout},
    {Name: "payment_timeout",      Weight: 12, Run: runPaymentTimeout},
    {Name: "inventory_error",      Weight: 10, Run: runInventoryError},
    {Name: "slow_database",        Weight: 10, Run: runSlowDatabase},
    {Name: "retry_success",        Weight: 5,  Run: runRetrySuccess},
    {Name: "cache_miss_cascade",   Weight: 3,  Run: runCacheMissCascade},
}
```

**successful_checkout:** Normal 14-span trace. frontend→gateway→[inventory+payment] parallel. ~280ms total.

**payment_timeout:** stripe.charge span takes 3100ms, api-gateway has 3s timeout → ERROR span "payment_timeout". 3 spans with errors.

**inventory_error:** inventory DB returns 500, propagates up as error to api-gateway root span.

**slow_database:** inventory DB query takes 800ms (table scan simulation). High latency, no error. Triggers latency-based tail sampler.

**retry_success:** payment fails on first attempt (span ERROR), retried, second succeeds. Shows duplicate CLIENT spans from api-gateway.

**cache_miss_cascade:** inventory cache miss → DB query cold → 400ms latency on what should be 20ms.

---

## API Specification

All endpoints at base URL `http://localhost:4318`.

### Ingest

**`POST /api/v1/spans`** — native JSON batch format
```json
{
  "spans": [{
    "traceId": "4bf92f3577b34da6a3ce929d0e0e4736",
    "spanId": "00f067aa0ba902b7",
    "parentSpanId": "",
    "name": "HTTP GET /api/users",
    "kind": 2,
    "serviceName": "frontend-svc",
    "serviceAttributes": {"host.name": "host-1"},
    "startTimeUnixNano": 1705312980000000000,
    "endTimeUnixNano":   1705312980045000000,
    "attributes": [
      {"key": "http.method", "stringValue": "GET"},
      {"key": "http.status_code", "intValue": 200}
    ],
    "events": [],
    "links": [],
    "status": {"code": 1, "message": ""}
  }]
}
```

**`POST /v1/traces`** — OTLP HTTP/JSON (OpenTelemetry-compatible)

### Query

**`GET /api/v1/traces`** — search
```
?service=payment-svc&operation=stripe.charge&minDuration=100&maxDuration=5000
&status=error&limit=20&offset=0&sortBy=duration&sortDesc=true
&startTime=1705312800000&endTime=1705316400000&tag=http.status_code=500
```

**`GET /api/v1/traces/:traceId`** — full trace
```json
{
  "traceId": "...",
  "spans": [{ "spanId": "...", "parentSpanId": "...", "name": "...", "serviceName": "...",
    "startTimeUnixNano": 0, "durationMs": 45.2, "kind": 2, "status": {"code": 1},
    "attributes": [], "events": [], "depth": 0, "hasError": false }],
  "criticalPath": ["spanId1", "spanId2", "spanId3"],
  "services": ["frontend-svc", "api-gateway", "payment-svc"],
  "durationMs": 312.4,
  "spanCount": 14,
  "errorCount": 0,
  "parallelGroups": [{"spanIds": ["id1","id2"], "startMs": 12.1, "endMs": 280.5}],
  "gaps": []
}
```

**`GET /api/v1/services`**
```json
{ "services": ["api-gateway", "frontend-svc", "inventory-svc", "payment-svc"] }
```

**`GET /api/v1/operations?service=payment-svc`**
```json
{ "operations": ["db.insert", "redis.get", "stripe.charge"] }
```

**`GET /api/v1/dependencies`**
```json
{
  "services": [
    { "name": "payment-svc", "errorRate": 0.052, "p99Ms": 290.4, "reqPerSec": 11.2 }
  ],
  "edges": [
    { "caller": "api-gateway", "callee": "payment-svc",
      "count": 5234, "errorCount": 271, "p99Ms": 286.1 }
  ]
}
```

**`GET /api/v1/traces/compare?base=:id&compare=:id`**
```json
{
  "durationDeltaMs": 145.2,
  "spanCountDelta": 2,
  "errorDelta": 1,
  "matched": [{"baseSpanId":"...","compareSpanId":"...","durationDeltaMs":12.1}],
  "onlyInBase": ["spanId"],
  "onlyInCompare": ["spanId"]
}
```

### Metrics

**`GET /api/v1/metrics/red`**
```json
{
  "metrics": [{
    "service": "payment-svc", "operation": "stripe.charge",
    "rate": 12.4, "errorRate": 0.61,
    "p50Ms": 95.2, "p95Ms": 185.0, "p99Ms": 290.1
  }]
}
```

**`GET /api/v1/metrics/heatmap?service=payment-svc&window=3600`**
```json
{
  "resolution": "10s",
  "buckets": [{ "ts": 1705312980, "spans": 124, "errors": 6 }]
}
```

### Sampler

**`GET /api/v1/sampler`**
```json
{
  "type": "adaptive",
  "config": { "targetRate": 100, "currentRate": 0.234, "minRate": 0.001 },
  "stats": { "sampledTotal": 45210, "droppedTotal": 148930,
    "samplingRate": 0.233, "observedThroughput": 97.4 }
}
```

**`PUT /api/v1/sampler`** — live sampler swap
```json
{ "type": "tail", "bufferTimeoutSec": 10,
  "policies": [
    { "type": "error" },
    { "type": "latency", "thresholdMs": 500 },
    { "type": "probabilistic", "rate": 0.05 }
  ]
}
```

### SSE Streams

**`GET /sse/spans`** — one event per span as it arrives
```
data: {"type":"span","traceId":"...","spanId":"...","service":"payment-svc","operation":"stripe.charge","durationMs":145,"hasError":false,"ts":"2024-01-15T10:23:45Z"}
```

**`GET /sse/traces`** — one event per completed trace
```
data: {"type":"trace","traceId":"...","durationMs":312,"spanCount":14,"rootService":"frontend-svc","services":["..."],"hasError":false}
```

**`GET /sse/metrics`** — live RED snapshot every 1 second
```
data: {"type":"metrics","service":"payment-svc","operation":"stripe.charge","rate":12.1,"errorRate":0.61,"p99Ms":291}
```

**`GET /sse/sampler`** — sampler stats every 5 seconds
```
data: {"type":"sampler","samplingRate":0.23,"observedThroughput":98.2,"sampledTotal":4521,"droppedTotal":14903}
```

---

## Frontend Page Specifications

### Search Page (`/`)

Filter bar: service dropdown, operation dropdown, status toggle (All/OK/Error), duration sliders, time range picker. All filters update query on change (debounced 300ms).

Trace list: card per trace showing service color-dots, root operation, duration bar (normalized), span count, time ago badge. Live: SSE `trace_complete` events prepend new cards with Framer Motion slide-in.

### Trace Detail Page (`/trace/:id`)

**D3 Waterfall** — the core visualization:

- SVG with zoom and pan (`d3.zoom()`)
- X-axis: time in ms from trace start (adaptive tick density)
- Y-axis: one row per span in depth-first order
- Row height: 24px, gap: 4px
- Service color palette: 10 distinct colors, consistent across all pages
- Critical path spans: 2px amber border overlay
- Error spans: `#dc2626` (red-600) background with subtle pulse animation
- Span kind pill inside each bar: SERVER/CLIENT/PRODUCER/CONSUMER
- Hover tooltip: service, op name, duration, start offset, status, attribute count
- Click span: opens SpanDrawer (shadcn Sheet from right)
- Minimap: fixed 120px-tall thumbnail at bottom showing full trace + viewport rect

**SpanDrawer contents:**
- Operation name, service badge
- Duration + start offset from trace start
- Span kind badge, status badge  
- Parent span link (click → scroll waterfall to parent)
- Attributes table: key / value / type columns
- Events sub-table: timestamp / name / attributes
- Links section: cross-trace references with click-to-open

**Left panel:** Service tree grouped by service name. Clicking expands/collapses. Each span listed with name + duration.

### Service Map (`/map`)

React Flow + dagre. Node = service. Edge = caller→callee.

**Node styling:** Circle, radius proportional to `log(spanCount)`. Color by error rate: `hsl(120 - errorRate*120, 70%, 45%)` (green → red). Label: service name + req/s + error%.

**Edge styling:** Stroke-width proportional to `log(requestCount+1)`. Stroke-opacity by recency. Label: P99 ms.

**Side panel:** Click node → show top 5 operations (table), 1min rate sparkline, error rate sparkline.

### Metrics Page (`/metrics`)

Service selector. Per-selected-service: three Recharts charts (Rate/line, ErrorRate/area-red, Latency-P50+P95+P99/multi-line). Below: operations table sortable by any metric column.

### Sampler Config Page (`/sampler`)

Current sampler card with live stats. Throughput chart: observed rate vs target rate. Policy builder for tail sampler. Live apply + confirmation diff.

### Compare Page (`/compare`)

Two waterfall charts synchronized by time axis. Left = base, right = compare. Color coded diff: amber = exists in both (duration delta in badge), green = only in compare, gray strikethrough = only in base. Summary diff row at top.

---

## Non-Goals (v1)

- Persistent storage / database backend
- Authentication
- Prometheus metrics export endpoint
- gRPC OTLP receiver
- Multi-node collector clustering
- Alert rules / anomaly notifications
