# Sampling Reference

Deep dive into all six sampling strategies: how each algorithm works, when to use it, known failure modes, and how to test correctness. This is the hardest part of the system to get right.

---

## Why Sampling Exists

At production scale, a large service might handle 100,000 requests/second. Recording a trace for every request would mean:
- 100,000 traces/sec × 10 spans/trace × 2KB/span = **2GB of span data per second**
- Ingestion overhead on every instrumented service
- Storage cost that grows with traffic

Sampling reduces this to a manageable volume while preserving enough data to understand system behavior. The core challenge is choosing WHICH traces to keep.

---

## The Fundamental Constraint: Consistency

In a distributed system, a single request spawns spans across multiple services. If service A decides to sample a trace but service B decides to drop it, you end up with a partial trace — useless for debugging.

**Rule:** All services MUST make the same sampling decision for the same trace.

This means sampling decisions cannot be made independently at random. They must be deterministic given the TraceID.

### The Wrong Way (do NOT do this)
```go
// WRONG: independent random decision
func (s *BadSampler) ShouldSample(p SamplingParameters) SamplingResult {
    if rand.Float64() < s.rate {
        return SamplingResult{Decision: Sample}
    }
    return SamplingResult{Decision: Drop}
}
```
Service A and Service B both call this with the same TraceID. Each gets a different random number. They disagree on ~`2*rate*(1-rate)` fraction of traces.

### The Right Way
```go
// CORRECT: deterministic from TraceID
func (s *ProbabilisticSampler) ShouldSample(p SamplingParameters) SamplingResult {
    // Use first 8 bytes of TraceID as a consistent hash
    id := binary.BigEndian.Uint64(p.TraceID[:8])
    if id < s.threshold {
        return SamplingResult{Decision: Sample}
    }
    return SamplingResult{Decision: Drop}
}
```
Both services hash the same TraceID → same value → same decision every time.

---

## 1. AlwaysSample / NeverSample

**Use case:** Development (AlwaysSample), feature flag disable (NeverSample).

No configuration. AlwaysSample returns `Sample` unconditionally. NeverSample returns `Drop`.

**Gotcha:** NeverSample is useful for high-volume internal operations (health checks, metrics scrapes) that you explicitly want to exclude. Combine with RuleBasedSampler:
```
Rule 1: operation="GET /health" → NeverSample
Rule 2: (default) → ProbabilisticSampler(0.1)
```

---

## 2. Probabilistic Sampler (TraceID-hash based)

**Algorithm:**
```
threshold = uint64(rate × 2^64)
id = first 8 bytes of TraceID as big-endian uint64
keep = (id < threshold)
```

**Why big-endian uint64 of first 8 bytes?** The TraceID is 16 bytes of random data. Any consistent transformation to a number works. We use the first 8 bytes as a uint64 for simplicity. The distribution is uniform (crypto/rand generates uniform random IDs).

**Threshold computation:**
```
rate = 0.1
threshold = uint64(0.1 × float64(math.MaxUint64))
          = uint64(0.1 × 18446744073709551615)
          = uint64(1844674407370955161.5)
          = 1844674407370955161

An ID is sampled iff id < 1844674407370955161
P(id < threshold) = threshold / 2^64 ≈ 0.1 ✓
```

**Rate accuracy:** With 100,000 traces, observed rate will be within ±0.3% of configured rate (binomial distribution, σ = sqrt(n×p×(1-p)) / n ≈ 0.003 for n=100k, p=0.1).

**Minimum meaningful rate:** 0.001 (0.1%). Below this, you might sample 0 traces in a short window.

**When to use:** When you need a fixed fraction of all traces regardless of content. The most common default sampler.

---

## 3. Rate-Limiting Sampler (Token Bucket)

**Algorithm:**
```
tokens = min(maxTokens, tokens + elapsed_seconds × tracesPerSec)
if tokens >= 1.0:
    tokens -= 1.0
    return Sample
else:
    return Drop
```

**Parameters:**
- `tracesPerSec`: target throughput (default 100)
- `maxTokens = tracesPerSec × 2`: burst capacity (allow 2× burst before throttling)

**Why token bucket?** It allows short bursts (important for traffic spikes), then smoothly throttles. A simple counter reset every second would cause synchronized dropping at second boundaries.

**Key property:** Rate-limiting does NOT use TraceID. Different services will make different decisions for the same trace. This violates the consistency rule.

**Correct usage:** Only deploy rate-limiting at the COLLECTOR (after spans arrive). Never at the instrumentation SDK. If you want rate-limited sampling at the service level, use ParentBased wrapping a rate limiter at the root only.

**When to use:** When you need absolute throughput guarantees regardless of traffic volume. "Never ingest more than 1000 traces/minute." Good for cost control.

---

## 4. Parent-Based Sampler

**Algorithm:**
```
if span has no parent:
    use root_sampler.ShouldSample(params)
elif parent was sampled (from propagation headers):
    use remoteParentSampled.ShouldSample(params)    // usually AlwaysSample
else:
    use remoteParentNotSampled.ShouldSample(params) // usually NeverSample
```

**Why it exists:** Once the root span decides to sample, ALL child spans across ALL services must also be sampled (to have a complete trace). The parent-based sampler propagates the root's decision.

**Typical configuration:**
```go
ParentBasedSampler{
    root: ProbabilisticSampler(0.1),       // 10% of new traces
    remoteParentSampled: AlwaysSampler{},  // if parent sampled → always sample
    remoteParentNotSampled: NeverSampler{},// if parent not sampled → never sample
}
```

**The root decision is in the propagation headers:** `traceparent: 00-{id}-{spanId}-01` (last byte 01 = sampled). When service B receives a request with sampled=1, the ParentBased sampler sees `ParentSampled = true` and uses `remoteParentSampled` (AlwaysSample).

**When to use:** Always, as your default production sampler. Wrap whatever root sampler you want inside it.

---

## 5. Adaptive Sampler

**Goal:** Maintain a target throughput (e.g., 100 traces/second) regardless of actual traffic volume. If traffic doubles, halve the sampling rate. If traffic drops, raise the rate.

**Algorithm:**
```
Every adjustPeriod (default 5s):
    observed = slidingWindow.Rate()  // traces/sec in last 60s
    if observed == 0: return
    ratio = targetRate / observed
    deviation = |ratio - 1.0|
    if deviation < 0.10: return  // within 10%, don't adjust
    deviations++
    if deviations < 3: return    // hysteresis: require 3 windows of deviation
    newRate = currentRate × ratio
    newRate = clamp(newRate, minRate=0.001, maxRate=1.0)
    inner.SetRate(newRate)
    deviations = 0
```

**Why hysteresis?** Without it, the sampler oscillates. If traffic briefly spikes, the sampler drops its rate, then traffic returns to normal and the sampler raises it, then spikes again... A 3-window requirement prevents reaction to brief fluctuations.

**Why multiplicative adjustment?** Additive adjustment (add/subtract fixed amount) is slow to converge at extremes. Multiplicative adjustment is proportional: if observed rate is 2× target, cut in half. If observed rate is 0.5× target, double. This converges in O(log n) steps.

**Failure mode:** If traffic suddenly drops to 0 (service goes down), `observed = 0` and we skip adjustment (division by zero protection). Rate stays at last value.

**When to use:** Production systems where traffic volume varies significantly (day/night patterns, traffic spikes). You want constant cost regardless of traffic.

---

## 6. Rule-Based Sampler

**Algorithm:**
```
Sort rules by priority (descending)
For each rule:
    if rule matches params:
        return rule.Decision
Return fallback.ShouldSample(params)
```

**Rule matching:**
```go
func (r *Rule) Matches(p SamplingParameters) bool {
    if r.ServiceName != "" && r.ServiceName != p.ServiceName {
        return false
    }
    if r.OperationGlob != "" && !globMatch(r.OperationGlob, p.OperationName) {
        return false
    }
    if r.StatusCode != nil && p.StatusCode != nil && *r.StatusCode != *p.StatusCode {
        return false
    }
    for k, v := range r.Tags {
        found := false
        for _, attr := range p.Attributes {
            if attr.Key == k && attr.SVal == v {
                found = true
                break
            }
        }
        if !found { return false }
    }
    return true
}
```

**Glob matching:** Support `*` as wildcard only. `path.Match` handles this: `"HTTP GET *"` matches `"HTTP GET /api/users"`.

**Common rule configurations:**

*Always sample errors:*
```json
[
  {"service": "*", "status": "error", "decision": "sample", "priority": 100},
  {"service": "*", "decision": "probabilistic:0.05", "priority": 0}
]
```

*Exclude health checks:*
```json
[
  {"service": "*", "operation": "GET /health", "decision": "drop", "priority": 100},
  {"service": "*", "operation": "GET /metrics", "decision": "drop", "priority": 99},
  {"service": "*", "decision": "sample", "priority": 0}
]
```

*Different rates per service:*
```json
[
  {"service": "payment-svc", "decision": "sample", "priority": 100},
  {"service": "inventory-svc", "decision": "probabilistic:0.1", "priority": 90},
  {"service": "*", "decision": "probabilistic:0.01", "priority": 0}
]
```

**When to use:** When you have specific requirements about which traces to keep. "Always keep payment errors." "Never keep health checks." Works well combined with probabilistic fallback.

---

## 7. Tail-Based Sampler

**What's different:** All previous samplers make the sampling decision at span START time (head-based). The tail-based sampler buffers all spans and makes the decision at trace COMPLETION time, after seeing the full picture.

**Advantages:**
- Can keep ALL error traces (not just those that declared error at start)
- Can keep ALL slow traces (can measure total latency)
- Can drop boring successful traces

**Disadvantages:**
- Memory: must buffer all pending spans (potentially GBs at high volume)
- Latency: decision delayed by buffer timeout (10s by default)
- Complexity: must handle distributed completion (spans from 4 services)

**Buffer management:**
```
Span arrives for TraceID T:
    If T is in decided_set: apply decision immediately (drop or accept)
    Else: add to buffer[T]
         If buffer > maxSize: evict oldest trace (probabilistic drop)
         Reset quiet-period timer for T

After quiet_period (no new spans for T):
    Evaluate policies
    Move T to decided_set
    Accept or reject all buffered spans for T
```

**Quiet period vs absolute timeout:**
- Quiet period (our approach): wait N seconds after LAST span arrives. Good for: traces that take variable time (some finish in 100ms, some in 5s).
- Absolute timeout: wait N seconds after FIRST span arrives. Simpler but cuts off long traces.

We use quiet period (default 2s). This means a 1-second trace must wait 3 seconds total (1s trace + 2s quiet).

**Policy evaluation order matters:**
```
ErrorPolicy     → Always keep errors (highest priority, definitive YES)
LatencyPolicy   → Keep slow traces (definitive YES if slow)
ProbabilisticPolicy → Keep fraction of remaining traces (probabilistic YES or NO)
(implicit) → Drop if no policy decided YES
```

Policies return `(keep bool, reason string)`:
- `(true, "has_error")` = definitive YES, stop evaluating
- `(false, "")` = abstain, try next policy
- Only ProbabilisticPolicy ever returns `(false, "not_sampled")` (definitive NO)

**When to use:** When you want to guarantee visibility into ALL errors and ALL slow traces, which you can't do with head-based sampling (you don't know at start time if a trace will be slow or fail).

---

## Sampler Composition

The most powerful configuration combines multiple samplers:

```go
// Production-ready default:
sampler := ParentBasedSampler{
    root: RuleBasedSampler{
        rules: []Rule{
            // Always keep: errors from payment service
            {Service: "payment-svc", Status: StatusError, Decision: Sample, Priority: 100},
            // Never keep: health checks
            {OperationGlob: "GET /health*", Decision: Drop, Priority: 90},
        },
        fallback: AdaptiveSampler{
            TargetRate: 100,  // 100 traces/sec
            MinRate: 0.001,
            MaxRate: 1.0,
        },
    },
    remoteParentSampled: AlwaysSampler{},
    remoteParentNotSampled: NeverSampler{},
}
```

This says:
1. If parent decided: respect that decision
2. If new trace: check rules first
3. If no rule matches: use adaptive sampler to maintain 100 traces/sec
4. Payment service errors: always sample regardless of rate
5. Health checks: always drop

---

## Sampling Decision Propagation in Headers

When a service creates a root span and makes a sampling decision, it must communicate that decision to downstream services via propagation headers:

**Sampled (keep):**
```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
                                                                     ↑ 01 = sampled
```

**Not sampled (drop):**
```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00
                                                                     ↑ 00 = not sampled
```

Downstream services using `ParentBasedSampler` read this flag and act accordingly. This is how a single root decision propagates through the entire distributed system.

---

## Testing Sampling Correctness

### Required tests for each sampler

**ProbabilisticSampler:**
1. Rate accuracy: 100,000 samples at rate=0.1 → 10% ± 0.5%
2. Consistency: 10,000 unique TraceIDs, two sampler instances → always agree
3. Determinism: same TraceID always produces same decision
4. Rate=0.0: all dropped. Rate=1.0: all sampled.
5. Thread safety: 100 goroutines, same sampler instance, no races

**TailSampler:**
1. Error trace always kept (regardless of probabilistic rate=0.0)
2. Slow trace always kept (duration > latency threshold)
3. Normal trace dropped at rate=0.0 with no error/latency policies
4. Buffer eviction: insert maxSize+50 traces, verify maxSize is limit
5. Timeout: trace spans arrive, wait timeout+1s, verify evaluation happened
6. Stop(): all pending traces evaluated, no goroutine leaks

**RuleBasedSampler:**
1. High-priority rule matches before low-priority rule
2. No matching rule → fallback sampler used
3. Glob `"HTTP GET *"` matches `"HTTP GET /api/users"` and not `"HTTP POST /api/users"`
4. Tag matching: all specified tags must match

### Statistical tests

For probabilistic-style samplers, use binomial confidence intervals:

```go
// At rate p, after N observations, the 99% confidence interval for
// observed rate is: p ± 2.576 * sqrt(p*(1-p)/N)
// For p=0.1, N=100000: CI = 0.1 ± 0.00245 = [0.0976, 0.1025]

func assertSamplingRate(t *testing.T, sampler Sampler, rate float64, n int) {
    sampled := 0
    for i := 0; i < n; i++ {
        id, _ := NewTraceID()
        if sampler.ShouldSample(SamplingParameters{TraceID: id}).Decision == Sample {
            sampled++
        }
    }
    observed := float64(sampled) / float64(n)
    stdErr := math.Sqrt(rate * (1 - rate) / float64(n))
    delta := 3 * stdErr  // 99.7% confidence interval
    assert.InDelta(t, rate, observed, delta,
        "rate=%.4f, observed=%.4f, delta=%.4f", rate, observed, delta)
}
```

---

## Sampler Statistics to Track

The sampler endpoint (`GET /api/v1/sampler`) must report:

```go
type SamplerStats struct {
    // Counters (since start)
    SampledTotal int64
    DroppedTotal int64

    // Current rate
    SamplingRate          float64  // current probability being applied
    ObservedThroughputSec float64  // observed spans/sec (for adaptive display)

    // For tail sampler
    BufferedTraces  int
    EvaluatedTotal  int64
    KeptByError     int64
    KeptByLatency   int64
    KeptByRate      int64
    DroppedByRate   int64
}
```

These stats are shown on the Sampler Config page and streamed via SSE every 5 seconds. They let operators verify the sampler is working as expected.

---

## Live Sampler Switching

The system supports switching sampler types while running, without restart. The `PUT /api/v1/sampler` endpoint atomically swaps the sampler:

```go
type SamplerManager struct {
    mu      sync.RWMutex
    current Sampler
    stats   *SamplerStats
}

func (m *SamplerManager) Swap(newSampler Sampler) {
    m.mu.Lock()
    old := m.current
    m.current = newSampler
    m.mu.Unlock()
    
    // Stop background goroutines of old sampler if applicable
    if s, ok := old.(interface{ Stop() }); ok {
        s.Stop()
    }
}

func (m *SamplerManager) ShouldSample(p SamplingParameters) SamplingResult {
    m.mu.RLock()
    s := m.current
    m.mu.RUnlock()
    return s.ShouldSample(p)
}
```

Using `sync.RWMutex` allows concurrent calls to `ShouldSample` (which call `RLock`) while `Swap` uses the exclusive `Lock`. No blocking of in-flight sampling decisions during swap.
