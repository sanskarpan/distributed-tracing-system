package sampler

import (
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// TailPolicy evaluates a complete trace and decides whether to keep it.
type TailPolicy interface {
	Name() string
	// Evaluate returns (keep, reason). If reason == "", the policy abstains.
	Evaluate(spans []*model.Span, duration time.Duration) (bool, string)
}

// ErrorPolicy keeps traces that have any span with StatusError.
type ErrorPolicy struct{}

func (ErrorPolicy) Name() string { return "error" }
func (ErrorPolicy) Evaluate(spans []*model.Span, _ time.Duration) (bool, string) {
	for _, s := range spans {
		if s.Status.Code == model.StatusError {
			return true, "has_error"
		}
	}
	return false, ""
}

// LatencyPolicy keeps traces whose duration exceeds the threshold.
type LatencyPolicy struct {
	Threshold time.Duration
}

func (l LatencyPolicy) Name() string { return "latency" }
func (l LatencyPolicy) Evaluate(_ []*model.Span, duration time.Duration) (bool, string) {
	if duration >= l.Threshold {
		return true, "high_latency"
	}
	return false, ""
}

// ProbabilisticTailPolicy keeps a fraction of traces based on TraceID hash.
type ProbabilisticTailPolicy struct {
	sampler *ProbabilisticSampler
}

func NewProbabilisticTailPolicy(rate float64) *ProbabilisticTailPolicy {
	return &ProbabilisticTailPolicy{sampler: NewProbabilistic(rate)}
}

func (p *ProbabilisticTailPolicy) Name() string { return "probabilistic" }
func (p *ProbabilisticTailPolicy) Evaluate(spans []*model.Span, _ time.Duration) (bool, string) {
	if len(spans) == 0 {
		return false, "no_spans"
	}
	result := p.sampler.ShouldSample(SamplingParameters{TraceID: spans[0].TraceID})
	if result.Decision == Sample {
		return true, "probabilistic_sample"
	}
	return false, "probabilistic_drop"
}

type tailBuffer struct {
	spans   []*model.Span
	firstAt time.Time
	timer   *time.Timer
}

// TailSampler buffers spans until a trace is complete, then evaluates policies.
type TailSampler struct {
	mu       sync.Mutex
	buffer   map[model.TraceID]*tailBuffer
	decided  map[model.TraceID]bool // true=accept, false=reject
	timeout  time.Duration
	maxSize  int
	policies []TailPolicy
	accept   func([]*model.Span)
	reject   func(model.TraceID)
}

// NewTailSampler creates a TailSampler with the given policies.
func NewTailSampler(timeout time.Duration, maxSize int, policies []TailPolicy,
	accept func([]*model.Span), reject func(model.TraceID)) *TailSampler {
	return &TailSampler{
		buffer:   make(map[model.TraceID]*tailBuffer),
		decided:  make(map[model.TraceID]bool),
		timeout:  timeout,
		maxSize:  maxSize,
		policies: policies,
		accept:   accept,
		reject:   reject,
	}
}

// AddSpan adds a span to the buffer. If the trace was already decided, applies immediately.
func (s *TailSampler) AddSpan(span *model.Span) {
	s.mu.Lock()

	if keep, decided := s.decided[span.TraceID]; decided {
		s.mu.Unlock()
		if keep && s.accept != nil {
			s.accept([]*model.Span{span})
		}
		return
	}

	buf, exists := s.buffer[span.TraceID]
	if !exists {
		if len(s.buffer) >= s.maxSize {
			s.evictOldestLocked()
		}
		buf = &tailBuffer{
			spans:   make([]*model.Span, 0, 8),
			firstAt: time.Now(),
		}
		s.buffer[span.TraceID] = buf
	}
	buf.spans = append(buf.spans, span)

	// Reset quiet-period timer on each new span
	if buf.timer != nil {
		buf.timer.Stop()
	}
	traceID := span.TraceID
	buf.timer = time.AfterFunc(s.timeout, func() {
		s.evaluateTrace(traceID)
	})
	s.mu.Unlock()
}

func (s *TailSampler) evictOldestLocked() {
	var oldest model.TraceID
	var oldestTime time.Time
	for id, buf := range s.buffer {
		if oldestTime.IsZero() || buf.firstAt.Before(oldestTime) {
			oldest = id
			oldestTime = buf.firstAt
		}
	}
	if buf, ok := s.buffer[oldest]; ok {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		delete(s.buffer, oldest)
		s.decided[oldest] = false
		if s.reject != nil {
			s.reject(oldest)
		}
	}
}

// recordDecisionLocked records a keep/reject decision and evicts the oldest half of the
// decided map if it exceeds 2× maxSize, preventing unbounded memory growth.
func (s *TailSampler) recordDecisionLocked(traceID model.TraceID, keep bool) {
	s.decided[traceID] = keep
	limit := s.maxSize * 2
	if len(s.decided) > limit {
		// Evict all entries — we can't easily sort by insertion order with a plain map,
		// so clear the entire decided set. Late-arriving spans for evicted traces will
		// be re-evaluated from scratch (conservative: they pass through the sampler again).
		s.decided = make(map[model.TraceID]bool)
		s.decided[traceID] = keep
	}
}

func (s *TailSampler) evaluateTrace(traceID model.TraceID) {
	s.mu.Lock()
	buf, ok := s.buffer[traceID]
	if !ok {
		s.mu.Unlock()
		return
	}
	spans := buf.spans
	delete(s.buffer, traceID)
	s.mu.Unlock()

	// Compute approximate duration
	var minStart, maxEnd time.Time
	for _, sp := range spans {
		if minStart.IsZero() || sp.StartTime.Before(minStart) {
			minStart = sp.StartTime
		}
		if maxEnd.IsZero() || sp.EndTime.After(maxEnd) {
			maxEnd = sp.EndTime
		}
	}
	var duration time.Duration
	if !minStart.IsZero() && !maxEnd.IsZero() {
		duration = maxEnd.Sub(minStart)
	}

	for _, policy := range s.policies {
		keep, reason := policy.Evaluate(spans, duration)
		if reason == "" {
			continue // abstain
		}
		s.mu.Lock()
		s.recordDecisionLocked(traceID, keep)
		s.mu.Unlock()
		if keep {
			if s.accept != nil {
				s.accept(spans)
			}
		} else {
			if s.reject != nil {
				s.reject(traceID)
			}
		}
		return
	}

	// No policy decided: default drop
	s.mu.Lock()
	s.recordDecisionLocked(traceID, false)
	s.mu.Unlock()
	if s.reject != nil {
		s.reject(traceID)
	}
}

// Stop flushes all pending traces and shuts down.
func (s *TailSampler) Stop() {
	s.mu.Lock()
	ids := make([]model.TraceID, 0, len(s.buffer))
	for id, buf := range s.buffer {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.evaluateTrace(id)
	}
}

// BufferedCount returns the number of traces currently buffered.
func (s *TailSampler) BufferedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buffer)
}

func (s *TailSampler) ShouldSample(p SamplingParameters) SamplingResult {
	// Tail sampler: head-based decision is always Sample (buffer everything)
	// The actual decision happens in evaluateTrace
	return SamplingResult{Decision: Sample, Reason: "tail: buffering"}
}

func (s *TailSampler) Name() string { return "tail" }
func (s *TailSampler) Config() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"timeoutSec":     s.timeout.Seconds(),
		"maxSize":        s.maxSize,
		"bufferedTraces": len(s.buffer),
	}
}
