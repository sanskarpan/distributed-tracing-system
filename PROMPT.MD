# Claude Code Master Prompt — Distributed Tracing System

Read this entire document. Then read `SPEC.md`, `CHECKLIST.md`, and `SAMPLING.md`. Build in the exact phase order defined in CHECKLIST.md. Do not write any code before reading all four documents.

---

## What You Are Building

A complete distributed tracing system — a Jaeger-lite that works with real OpenTelemetry context propagation. The system collects spans from instrumented services, reconstructs traces, applies intelligent sampling, and visualizes everything in a React frontend with a D3.js waterfall chart.

```
Demo Services (4 goroutines)
  └── generate realistic traces via DemoSDK
        └── POST /api/v1/spans or /v1/traces
              └── Ingestion Pipeline
                    ├── Sampler (6 strategies, live-switchable)
                    ├── Assembler (span → trace tree)
                    ├── Analysis (critical path, parallelism, gaps)
                    ├── Storage (in-memory, indexed, evicting)
                    └── SSE Broadcast → Frontend

Frontend
  ├── Search page (trace list + filters + live SSE)
  ├── Trace Detail (D3 waterfall + zoom + span drawer)
  ├── Service Map (React Flow dependency graph)
  ├── Metrics (RED: rate, errors, P50/P95/P99)
  ├── Sampler Config (live type switching + policy builder)
  └── Compare (side-by-side trace diff)
```

---

## Non-Negotiable Rules

1. **Phase order is mandatory.** `go test ./... -race` passes before advancing.
2. **Probabilistic sampler MUST use TraceID as input.** Not `rand.Float64()`. The entire point of distributed tracing is that all services agree on whether to sample a given trace. Verify this with the cross-service consistency test in Phase 3.
3. **D3.js for the waterfall, not Recharts.** The waterfall is the signature feature. It needs zoom/pan, per-row interaction, critical path overlay, and a minimap. Recharts cannot do this.
4. **No panics in library code.** Every sampler, storage, and analysis function returns errors.
5. **Demo services generate traces automatically from startup.** The frontend must show data immediately without any manual setup.
6. **All concurrent code passes `-race`.** Every shared structure is properly locked.
7. **TypeScript strict, no `any`.** `"strict": true` in tsconfig.

---

## Phase 0 — Setup

```bash
mkdir tracing && cd tracing
go mod init github.com/yourname/tracing
go get github.com/go-chi/chi/v5 github.com/stretchr/testify/assert github.com/stretchr/testify/require

mkdir -p cmd/{collector,demo}
mkdir -p internal/{model,collector,propagation,sampler,storage,processor,analysis,metrics,demo}
mkdir -p api

cd web
npm create vite@latest . -- --template react-ts
npm install d3 @types/d3 @xyflow/react @dagrejs/dagre @dagrejs/graphlib recharts framer-motion zustand ky tailwindcss @tailwindcss/vite
npx shadcn@latest init
npx shadcn@latest add button badge card tabs select slider switch separator scroll-area tooltip sheet dialog dropdown-menu
```

`Makefile`:
```makefile
.PHONY: build test race run demo dev lint

build:
	go build -o bin/collector ./cmd/collector
	go build -o bin/demo ./cmd/demo

test:
	go test ./... -v -count=1

race:
	go test ./... -race -count=1

run:
	go run ./cmd/collector &
	go run ./cmd/demo

dev:
	go run ./cmd/collector & go run ./cmd/demo & sleep 1 && (cd web && npm run dev)

lint:
	go vet ./...
	cd web && npx tsc --noEmit
```

---

## Phase 1 — IDs: The Foundation

IDs must be JSON-serializable as hex strings (not base64, not arrays):

```go
// correct JSON marshaling for TraceID
func (t TraceID) MarshalJSON() ([]byte, error) {
    return json.Marshal(t.String())
}

func (t *TraceID) UnmarshalJSON(b []byte) error {
    var s string
    if err := json.Unmarshal(b, &s); err != nil { return err }
    id, err := ParseTraceID(s)
    if err != nil { return err }
    *t = id
    return nil
}
```

**Write the uniqueness test first:**
```go
func TestTraceID_Uniqueness(t *testing.T) {
    seen := make(map[TraceID]struct{}, 10000)
    for i := 0; i < 10000; i++ {
        id, err := NewTraceID()
        require.NoError(t, err)
        _, exists := seen[id]
        assert.False(t, exists, "duplicate TraceID at iteration %d", i)
        seen[id] = struct{}{}
    }
}
```

---

## Phase 3 — The Critical Probabilistic Sampler Test

**Write this test BEFORE implementing the sampler.** It is the most important correctness test in the entire project.

```go
func TestProbabilisticSampler_CrossServiceConsistency(t *testing.T) {
    // Simulate two independent services using same traceID and same rate
    // They MUST agree on every trace, every time.
    rate := 0.3
    s1 := NewProbabilistic(rate)
    s2 := NewProbabilistic(rate)

    for i := 0; i < 10000; i++ {
        traceID, _ := NewTraceID()
        p := SamplingParameters{TraceID: traceID}

        result1 := s1.ShouldSample(p)
        result2 := s2.ShouldSample(p)

        assert.Equal(t, result1.Decision, result2.Decision,
            "services must agree on traceID %s: service1=%v service2=%v",
            traceID, result1.Decision, result2.Decision)
    }
}

func TestProbabilisticSampler_RateAccuracy(t *testing.T) {
    s := NewProbabilistic(0.1)
    sampled := 0
    total := 100_000
    for i := 0; i < total; i++ {
        id, _ := NewTraceID()
        if s.ShouldSample(SamplingParameters{TraceID: id}).Decision == Sample {
            sampled++
        }
    }
    rate := float64(sampled) / float64(total)
    // Allow ±0.5% deviation from target 10%
    assert.InDelta(t, 0.10, rate, 0.005,
        "sampling rate should be ~10%%, got %.4f", rate)
}
```

**Why TraceID-based matters:** If service A uses `rand.Float64()` and service B uses `rand.Float64()`, they will disagree on 18% of traces (at 10% rate: P(disagree) = 0.1*0.9*2 = 18%). You'd have traces where the first span is sampled but child spans from another service are dropped — you can never reconstruct the full trace.

---

## Phase 6 — Tail Sampler: The Buffer Management

The tail sampler is the most complex component. The key challenge is that spans arrive continuously and you don't know when a trace is "complete."

```go
func (s *TailSampler) AddSpan(span *Span) {
    s.mu.Lock()
    
    // Check if this trace was already decided
    if _, decided := s.decided[span.TraceID]; decided {
        // Already dropped or accepted — apply immediately
        s.mu.Unlock()
        return
    }
    
    buf, exists := s.buffer[span.TraceID]
    if !exists {
        if len(s.buffer) >= s.maxSize {
            // Evict oldest buffered trace
            s.evictOldestLocked()
        }
        buf = &tailBuffer{
            spans:   make([]*Span, 0, 8),
            firstAt: time.Now(),
        }
        // Timer fires after s.timeout; triggers evaluation
        buf.timer = time.AfterFunc(s.timeout, func() {
            s.evaluateTrace(span.TraceID)
        })
        s.buffer[span.TraceID] = buf
    }
    buf.spans = append(buf.spans, span)
    s.mu.Unlock()
    
    // Reset timer on each new span (trace still active)
    // Do NOT reset if timer already fired
}

func (s *TailSampler) evaluateTrace(traceID TraceID) {
    s.mu.Lock()
    buf, ok := s.buffer[traceID]
    if !ok { s.mu.Unlock(); return }
    spans := buf.spans
    delete(s.buffer, traceID)
    s.mu.Unlock()
    
    // Compute approximate duration for latency policy
    var minStart, maxEnd time.Time
    for _, sp := range spans {
        if minStart.IsZero() || sp.StartTime.Before(minStart) { minStart = sp.StartTime }
        if maxEnd.IsZero() || sp.EndTime.After(maxEnd)        { maxEnd = sp.EndTime }
    }
    duration := maxEnd.Sub(minStart)
    
    // Apply policies in order
    for _, policy := range s.policies {
        keep, reason := policy.Evaluate(spans, duration)
        if keep {
            s.accept(spans, reason)
            return
        }
        if !keep && reason != "" {
            // Explicit drop decision from this policy
            s.reject(traceID, reason)
            return
        }
        // reason == "" means "no decision, try next policy"
    }
    
    // No policy decided: default drop
    s.reject(traceID, "no matching policy")
}
```

**Policy contract:** A policy returns `(keep bool, reason string)`. If `reason == ""`, the policy abstains (try next). If `reason != ""`, the policy made a definitive decision (stop).

---

## Phase 7 — Assembler: Out-of-Order Spans

Spans from distributed services arrive out of order. The assembler must handle this:

```go
type Assembler struct {
    mu      sync.Mutex
    pending map[TraceID]*pendingTrace
    timeout time.Duration   // 2s quiet period
    onComplete func(*Trace)  // callback when trace assembled
}

type pendingTrace struct {
    spans   []*Span
    timer   *time.Timer
    lastAt  time.Time
}

func (a *Assembler) AddSpan(span *Span) {
    a.mu.Lock()
    
    pt, ok := a.pending[span.TraceID]
    if !ok {
        pt = &pendingTrace{spans: make([]*Span, 0, 8), lastAt: time.Now()}
        a.pending[span.TraceID] = pt
    }
    pt.spans = append(pt.spans, span)
    pt.lastAt = time.Now()
    
    // Reset quiet timer on each new span
    if pt.timer != nil {
        pt.timer.Stop()
    }
    pt.timer = time.AfterFunc(a.timeout, func() {
        a.finalize(span.TraceID)
    })
    a.mu.Unlock()
}

func (a *Assembler) finalize(traceID TraceID) {
    a.mu.Lock()
    pt, ok := a.pending[traceID]
    if !ok { a.mu.Unlock(); return }
    spans := pt.spans
    delete(a.pending, traceID)
    a.mu.Unlock()
    
    trace := a.buildTree(spans)
    a.onComplete(trace)
}

func (a *Assembler) buildTree(spans []*Span) *Trace {
    spanMap := make(map[SpanID]*Span, len(spans))
    for _, s := range spans { spanMap[s.SpanID] = s }
    
    var root *Span
    for _, s := range spans {
        if s.IsRoot() {
            root = s
            continue
        }
        parent, ok := spanMap[s.ParentSpanID]
        if ok {
            parent.Children = append(parent.Children, s)
            s.Depth = parent.Depth + 1
        }
        // Orphan spans: keep in trace but not attached to tree
    }
    
    // Sort children by start time for consistent ordering
    for _, s := range spans {
        sort.Slice(s.Children, func(i, j int) bool {
            return s.Children[i].StartTime.Before(s.Children[j].StartTime)
        })
    }
    
    trace := &Trace{
        TraceID:  spans[0].TraceID,
        Spans:    spans,
        RootSpan: root,
    }
    computeTraceMetadata(trace)
    return trace
}
```

---

## Phase 8 — OTLP Receiver: Correct Timestamp Parsing

OpenTelemetry timestamps are nanoseconds since Unix epoch. JavaScript Date.now() returns milliseconds. Keep everything in Go as `time.Time`:

```go
func nanoToTime(ns uint64) time.Time {
    if ns == 0 { return time.Time{} }
    return time.Unix(0, int64(ns))
}

// OTLP attribute value parsing
func parseOTLPValue(v map[string]any) KeyValue {
    if sv, ok := v["stringValue"].(string); ok {
        return KeyValue{Type: ValueString, SVal: sv}
    }
    if iv, ok := v["intValue"]; ok {
        switch x := iv.(type) {
        case float64: return KeyValue{Type: ValueInt, IVal: int64(x)}
        case string:
            n, _ := strconv.ParseInt(x, 10, 64)
            return KeyValue{Type: ValueInt, IVal: n}
        }
    }
    if bv, ok := v["boolValue"].(bool); ok {
        return KeyValue{Type: ValueBool, BVal: bv}
    }
    if dv, ok := v["doubleValue"].(float64); ok {
        return KeyValue{Type: ValueFloat, FVal: dv}
    }
    return KeyValue{}
}
```

**OTLP service name extraction:**
```go
// The service name is in resourceSpans[].resource.attributes[]
// where attribute.key == "service.name"
func extractServiceName(attrs []map[string]any) string {
    for _, attr := range attrs {
        if key, _ := attr["key"].(string); key == "service.name" {
            if val, ok := attr["value"].(map[string]any); ok {
                if sv, ok := val["stringValue"].(string); ok {
                    return sv
                }
            }
        }
    }
    return "unknown"
}
```

---

## Phase 16 — D3 Waterfall: Complete Implementation Guide

The waterfall is the most important component. Build it carefully.

```typescript
// web/src/components/waterfall/WaterfallChart.tsx
import * as d3 from 'd3';
import { useEffect, useRef } from 'react';
import { TraceDetail, SpanRow, buildSpanRows } from './utils';

const LABEL_WIDTH  = 220;
const ROW_HEIGHT   = 24;
const ROW_GAP      = 4;
const ROW_STRIDE   = ROW_HEIGHT + ROW_GAP;
const PADDING_TOP  = 32;  // time axis height
const PADDING_LEFT = 0;
const MINIMAP_H    = 80;

export function WaterfallChart({ trace, onSpanSelect, criticalPathIds }: Props) {
  const svgRef = useRef<SVGSVGElement>(null);
  const minimapRef = useRef<SVGSVGElement>(null);
  const zoomRef = useRef<d3.ZoomBehavior<SVGSVGElement, unknown>>();

  useEffect(() => {
    if (!svgRef.current || !trace) return;
    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();  // clean re-render

    const rows = buildSpanRows(trace);  // depth-first ordered
    const totalDurationMs = trace.durationMs;
    const svgWidth = svgRef.current.clientWidth || 1200;
    const chartWidth = svgWidth - LABEL_WIDTH;
    const svgHeight = PADDING_TOP + rows.length * ROW_STRIDE;

    svg.attr('height', svgHeight + MINIMAP_H + 16);

    // ── X scale ─────────────────────────────────────────────────────
    const xScale = d3.scaleLinear()
      .domain([0, totalDurationMs])
      .range([LABEL_WIDTH, svgWidth]);

    // ── Time axis ───────────────────────────────────────────────────
    const axisG = svg.append('g')
      .attr('class', 'time-axis')
      .attr('transform', `translate(0, ${PADDING_TOP})`);

    const updateAxis = (scale: d3.ScaleLinear<number, number>) => {
      axisG.call(
        d3.axisTop(scale)
          .tickFormat(d => `${d}ms`)
          .ticks(Math.floor(chartWidth / 80))
      );
    };
    updateAxis(xScale);

    // ── Span rows ────────────────────────────────────────────────────
    const rowsG = svg.append('g').attr('class', 'span-rows');

    const rowGs = rowsG.selectAll('.span-row')
      .data(rows)
      .enter()
      .append('g')
      .attr('class', d => `span-row ${d.hasError ? 'has-error' : ''}`)
      .attr('transform', (_, i) => `translate(0, ${PADDING_TOP + i * ROW_STRIDE})`);

    // Service label
    rowGs.append('text')
      .attr('class', 'service-label')
      .attr('x', d => LABEL_WIDTH - 8 + d.depth * 12)
      .attr('y', ROW_HEIGHT / 2)
      .attr('text-anchor', 'end')
      .attr('dominant-baseline', 'central')
      .attr('fill', d => getServiceColor(d.serviceName))
      .attr('font-size', 12)
      .text(d => truncate(d.name, 24));

    // Span bar background
    rowGs.append('rect')
      .attr('class', 'span-bg')
      .attr('x', d => xScale(d.startMs))
      .attr('y', 0)
      .attr('width', d => Math.max(2, xScale(d.startMs + d.durationMs) - xScale(d.startMs)))
      .attr('height', ROW_HEIGHT)
      .attr('rx', 3)
      .attr('fill', d => d.hasError ? '#dc2626' : getServiceColor(d.serviceName))
      .attr('fill-opacity', d => d.hasError ? 0.9 : 0.7)
      .attr('stroke', d => criticalPathIds.has(d.spanId) ? '#f59e0b' : 'none')
      .attr('stroke-width', 2)
      .style('cursor', 'pointer')
      .on('mouseenter', function(event, d) { showTooltip(event, d); })
      .on('mouseleave', () => hideTooltip())
      .on('click', (_, d) => onSpanSelect(d));

    // Duration label inside bar
    rowGs.append('text')
      .attr('class', 'duration-label')
      .attr('x', d => xScale(d.startMs) + 4)
      .attr('y', ROW_HEIGHT / 2)
      .attr('dominant-baseline', 'central')
      .attr('font-size', 11)
      .attr('fill', 'white')
      .text(d => {
        const barW = xScale(d.startMs + d.durationMs) - xScale(d.startMs);
        return barW > 60 ? `${d.durationMs.toFixed(1)}ms` : '';
      });

    // ── Zoom ─────────────────────────────────────────────────────────
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.5, 50])
      .translateExtent([[LABEL_WIDTH, 0], [svgWidth, svgHeight]])
      .on('zoom', (event) => {
        const newXScale = event.transform.rescaleX(xScale);
        updateAxis(newXScale);
        
        rowGs.selectAll('.span-bg')
          .attr('x', (d: SpanRow) => newXScale(d.startMs))
          .attr('width', (d: SpanRow) => Math.max(2, newXScale(d.startMs + d.durationMs) - newXScale(d.startMs)));
        
        rowGs.selectAll('.duration-label')
          .attr('x', (d: SpanRow) => newXScale(d.startMs) + 4)
          .text((d: SpanRow) => {
            const barW = newXScale(d.startMs + d.durationMs) - newXScale(d.startMs);
            return barW > 60 ? `${d.durationMs.toFixed(1)}ms` : '';
          });
      });

    svg.call(zoom);
    zoomRef.current = zoom;

  }, [trace, criticalPathIds]);

  return (
    <div className="relative w-full overflow-hidden">
      <svg ref={svgRef} className="w-full" />
    </div>
  );
}
```

**`buildSpanRows` utility:**
```typescript
export function buildSpanRows(trace: TraceDetail): SpanRow[] {
  const rows: SpanRow[] = [];
  const traceStart = Math.min(...trace.spans.map(s => s.startTimeUnixNano)) / 1e6;
  
  function visit(span: SpanDTO, depth: number) {
    rows.push({
      spanId: span.spanId,
      name: span.name,
      serviceName: span.serviceName,
      startMs: span.startTimeUnixNano / 1e6 - traceStart,
      durationMs: span.durationMs,
      depth,
      hasError: span.status.code === 2,
      kind: span.kind,
      span,
    });
    // Visit children in startTime order
    const children = trace.spans
      .filter(s => s.parentSpanId === span.spanId)
      .sort((a, b) => a.startTimeUnixNano - b.startTimeUnixNano);
    for (const child of children) {
      visit(child, depth + 1);
    }
  }
  
  const root = trace.spans.find(s => !s.parentSpanId || s.parentSpanId === '');
  if (root) visit(root, 0);
  
  // Add orphan spans that weren't visited
  const visited = new Set(rows.map(r => r.spanId));
  for (const s of trace.spans) {
    if (!visited.has(s.spanId)) {
      rows.push({ ...makeRow(s, 0) });
    }
  }
  
  return rows;
}
```

**Service color palette (10 distinct, accessible):**
```typescript
const SERVICE_COLORS = [
  '#2563eb', // blue-600
  '#059669', // emerald-600
  '#d97706', // amber-600
  '#7c3aed', // violet-600
  '#db2777', // pink-600
  '#0891b2', // cyan-600
  '#65a30d', // lime-600
  '#dc2626', // red-600 (only for error services)
  '#4f46e5', // indigo-600
  '#0d9488', // teal-600
];

const serviceColorMap = new Map<string, string>();
let colorIndex = 0;

export function getServiceColor(service: string): string {
  if (!serviceColorMap.has(service)) {
    serviceColorMap.set(service, SERVICE_COLORS[colorIndex % SERVICE_COLORS.length]);
    colorIndex++;
  }
  return serviceColorMap.get(service)!;
}
```

---

## Phase 12 — Demo Services: Realistic Trace Generation

The demo services must produce traces that demonstrate every UI feature. Here is the successful_checkout scenario:

```go
// internal/demo/scenarios.go
func runSuccessfulCheckout(sdk *DemoSDK) error {
    ctx := context.Background()
    
    // 1. Frontend span
    ctx, frontendSpan := sdk.NewSpan(ctx, "HTTP GET /checkout", SpanKindServer,
        WithService("frontend-svc"))
    frontendSpan.AddAttr(StringKV("http.method", "GET"))
    frontendSpan.AddAttr(StringKV("http.url", "/checkout"))
    frontendSpan.AddAttr(IntKV("http.status_code", 200))
    time.Sleep(jitter(5, 15)) // frontend processing
    
    // 2. API Gateway span (new service, extract parent from headers)
    gwCtx, gwSpan := sdk.NewChildSpan(ctx, "GET /api/checkout", SpanKindServer,
        WithService("api-gateway"))
    time.Sleep(jitter(2, 5))
    
    // 3. Parallel calls to inventory and payment
    var wg sync.WaitGroup
    var inventoryErr, paymentErr error
    
    wg.Add(2)
    go func() {
        defer wg.Done()
        invCtx, invClientSpan := sdk.NewChildSpan(gwCtx, "HTTP GET inventory", SpanKindClient,
            WithService("api-gateway"))
        invClientSpan.AddAttr(StringKV("peer.service", "inventory-svc"))
        time.Sleep(jitter(2, 5))
        
        // Inventory service spans
        invSvcCtx, invSvcSpan := sdk.NewChildSpan(invCtx, "GET /api/inventory", SpanKindServer,
            WithService("inventory-svc"))
        
        _, cacheSpan := sdk.NewChildSpan(invSvcCtx, "cache.get", SpanKindInternal,
            WithService("inventory-svc"))
        cacheSpan.AddAttr(StringKV("db.system", "redis"))
        cacheSpan.AddAttr(StringKV("cache.hit", "true"))
        time.Sleep(jitter(1, 3))
        sdk.Finish(cacheSpan)
        
        _, dbSpan := sdk.NewChildSpan(invSvcCtx, "db.query", SpanKindInternal,
            WithService("inventory-svc"))
        dbSpan.AddAttr(StringKV("db.system", "postgresql"))
        dbSpan.AddAttr(StringKV("db.statement", "SELECT stock FROM inventory WHERE product_id = $1"))
        time.Sleep(jitter(5, 15))
        sdk.Finish(dbSpan)
        
        sdk.Finish(invSvcSpan)
        sdk.Finish(invClientSpan)
    }()
    
    go func() {
        defer wg.Done()
        // ... similar for payment service with redis.get + stripe.charge + db.insert
    }()
    
    wg.Wait()
    sdk.Finish(gwSpan)
    sdk.Finish(frontendSpan)
    
    return sdk.Export() // batch export to collector
}
```

**Key properties of the demo:**
- Uses `context.Context` to thread parent spans through goroutines
- `sync.WaitGroup` for the parallel calls → creates genuine parallel spans in trace
- Random jitter makes each trace unique (different durations)
- The payment_timeout scenario uses a 3.1s sleep for stripe.charge + sets error status + records "timeout" event

---

## SSE Implementation: Client Disconnect Handling

```go
func (h *Handler) ServeSSESpans(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", 500)
        return
    }
    
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
    
    // Register this client with the SSE bus
    ch := h.sseBus.Subscribe()
    defer h.sseBus.Unsubscribe(ch)
    
    // Send initial ping so client knows connection is live
    fmt.Fprintf(w, "data: {\"type\":\"ping\"}\n\n")
    flusher.Flush()
    
    for {
        select {
        case event, ok := <-ch:
            if !ok { return }  // channel closed (server shutdown)
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        case <-r.Context().Done():  // client disconnected
            return
        }
    }
}
```

**SSE bus with per-connection buffered channels:**
```go
type SSEBus struct {
    mu   sync.RWMutex
    subs map[chan SSEEvent]struct{}
}

func (b *SSEBus) Subscribe() chan SSEEvent {
    ch := make(chan SSEEvent, 100)  // buffered: don't block publisher
    b.mu.Lock()
    b.subs[ch] = struct{}{}
    b.mu.Unlock()
    return ch
}

func (b *SSEBus) Unsubscribe(ch chan SSEEvent) {
    b.mu.Lock()
    delete(b.subs, ch)
    b.mu.Unlock()
}

func (b *SSEBus) Broadcast(event SSEEvent) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for ch := range b.subs {
        select {
        case ch <- event:  // non-blocking send
        default:           // slow client: drop event (don't block)
        }
    }
}
```

---

## Service Map: React Flow + Dagre

```tsx
// web/src/components/servicemap/ServiceMapGraph.tsx
import ReactFlow, { Node, Edge, Background, Controls } from '@xyflow/react';
import dagre from '@dagrejs/dagre';

const NODE_W = 160, NODE_H = 80;

function layoutGraph(nodes: Node[], edges: Edge[]) {
    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({ rankdir: 'LR', nodesep: 60, ranksep: 100 });
    nodes.forEach(n => g.setNode(n.id, { width: NODE_W, height: NODE_H }));
    edges.forEach(e => g.setEdge(e.source, e.target));
    dagre.layout(g);
    return nodes.map(n => {
        const pos = g.node(n.id);
        return { ...n, position: { x: pos.x - NODE_W/2, y: pos.y - NODE_H/2 } };
    });
}

function errorRateToColor(rate: number): string {
    // HSL: 120=green, 60=yellow, 0=red
    const hue = Math.max(0, 120 - rate * 1200);  // 0.1 → 0 (red), 0.01 → 108 (near green)
    return `hsl(${hue}, 70%, 45%)`;
}
```

---

## Correctness Tests to Never Skip

1. **Probabilistic sampler cross-service consistency** (Phase 3) — same traceID must always produce same decision from independent sampler instances.

2. **Assembler out-of-order delivery** — add parent span last, verify tree is built correctly.

3. **Tail sampler ErrorPolicy** — span with StatusError → ErrorPolicy.Evaluate returns `(true, "has_error")`.

4. **W3C propagation roundtrip** — inject into headers, extract from headers, verify SpanContext is identical.

5. **TPC-equivalent: trace invariant** — after 1000 demo traces, every trace in storage has correct `Services` field (all services that appear in its spans are listed).

6. **Memory store eviction** — insert maxSize+100 traces, verify total stored == maxSize, oldest traces gone.

---

## Definition of Done

1. `go test ./... -race -count=1` — zero failures, zero races
2. `cd web && npx tsc --noEmit` — zero type errors
3. Within 5 seconds of `make dev`, the Search page shows live traces being added (demo runner producing data)
4. Clicking any trace opens the waterfall with correct span ordering and critical path highlighted
5. Switching to `/map` shows service dependency graph with edges
6. `PUT /api/v1/sampler` with `{"type":"probabilistic","rate":0.1}` → subsequent GET shows ~10% sampling rate in stats
7. Tail sampler: `payment_timeout` scenario traces appear despite 5% probabilistic rate (because ErrorPolicy keeps them)
8. D3 waterfall: zoom in 10x on a long trace → bars expand, time axis rescales
9. Compare page: two `successful_checkout` traces show similar structure with duration deltas
10. `make build` produces working `bin/collector` and `bin/demo` binaries
