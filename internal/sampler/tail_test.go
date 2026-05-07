package sampler_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

func makeSpan(traceID model.TraceID, hasError bool, duration time.Duration) *model.Span {
	spanID, _ := model.NewSpanID()
	sp := &model.Span{
		TraceID:   traceID,
		SpanID:    spanID,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(duration),
	}
	if hasError {
		sp.Status.Code = model.StatusError
	}
	return sp
}

func TestTailSampler_ErrorPolicyKeepsTrace(t *testing.T) {
	var mu sync.Mutex
	accepted := 0
	rejected := 0

	s := sampler.NewTailSampler(
		50*time.Millisecond, 1000,
		[]sampler.TailPolicy{sampler.ErrorPolicy{}},
		func(spans []*model.Span) {
			mu.Lock()
			accepted++
			mu.Unlock()
		},
		func(id model.TraceID) {
			mu.Lock()
			rejected++
			mu.Unlock()
		},
	)

	traceID, _ := model.NewTraceID()
	s.AddSpan(makeSpan(traceID, true, 10*time.Millisecond))

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	a, r := accepted, rejected
	mu.Unlock()
	assert.Equal(t, 1, a, "error trace should be accepted")
	assert.Equal(t, 0, r)
}

func TestTailSampler_LatencyPolicyKeepsSlowTrace(t *testing.T) {
	var mu sync.Mutex
	accepted := 0

	s := sampler.NewTailSampler(
		50*time.Millisecond, 1000,
		[]sampler.TailPolicy{
			sampler.LatencyPolicy{Threshold: 100 * time.Millisecond},
		},
		func(spans []*model.Span) {
			mu.Lock()
			accepted++
			mu.Unlock()
		},
		nil,
	)

	traceID, _ := model.NewTraceID()
	s.AddSpan(makeSpan(traceID, false, 500*time.Millisecond))

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	a := accepted
	mu.Unlock()
	assert.Equal(t, 1, a)
}

func TestTailSampler_NoMatch_DefaultDrop(t *testing.T) {
	var mu sync.Mutex
	rejected := 0

	s := sampler.NewTailSampler(
		50*time.Millisecond, 1000,
		[]sampler.TailPolicy{
			sampler.LatencyPolicy{Threshold: 10 * time.Second}, // very high threshold
		},
		nil,
		func(id model.TraceID) {
			mu.Lock()
			rejected++
			mu.Unlock()
		},
	)

	traceID, _ := model.NewTraceID()
	s.AddSpan(makeSpan(traceID, false, 1*time.Millisecond))

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	r := rejected
	mu.Unlock()
	assert.Equal(t, 1, r)
}

func TestTailSampler_MaxSizeEviction(t *testing.T) {
	evicted := 0
	var mu sync.Mutex

	s := sampler.NewTailSampler(
		10*time.Second, 10, // very long timeout so they stay buffered; small maxSize
		[]sampler.TailPolicy{},
		nil,
		func(id model.TraceID) {
			mu.Lock()
			evicted++
			mu.Unlock()
		},
	)

	// Insert maxSize+5 traces
	for i := 0; i < 15; i++ {
		traceID, _ := model.NewTraceID()
		s.AddSpan(makeSpan(traceID, false, 10*time.Millisecond))
	}

	// At any time, at most maxSize traces are buffered
	count := s.BufferedCount()
	assert.LessOrEqual(t, count, 10)
}

func TestTailSampler_AcceptCallbackGetsSpans(t *testing.T) {
	var mu sync.Mutex
	var acceptedSpans []*model.Span

	s := sampler.NewTailSampler(
		50*time.Millisecond, 1000,
		[]sampler.TailPolicy{sampler.ErrorPolicy{}},
		func(spans []*model.Span) {
			mu.Lock()
			acceptedSpans = append(acceptedSpans, spans...)
			mu.Unlock()
		},
		nil,
	)

	traceID, _ := model.NewTraceID()
	span1 := makeSpan(traceID, true, 10*time.Millisecond)
	s.AddSpan(span1)

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, acceptedSpans, 1)
	assert.Equal(t, span1.SpanID, acceptedSpans[0].SpanID)
}

func TestTailSampler_Stop_FlushPending(t *testing.T) {
	var mu sync.Mutex
	rejected := 0

	s := sampler.NewTailSampler(
		10*time.Second, 1000, // long timeout — won't fire before Stop()
		[]sampler.TailPolicy{
			sampler.LatencyPolicy{Threshold: 10 * time.Second},
		},
		nil,
		func(id model.TraceID) {
			mu.Lock()
			rejected++
			mu.Unlock()
		},
	)

	for i := 0; i < 5; i++ {
		traceID, _ := model.NewTraceID()
		s.AddSpan(makeSpan(traceID, false, 1*time.Millisecond))
	}

	s.Stop()

	mu.Lock()
	r := rejected
	mu.Unlock()
	assert.Equal(t, 5, r, "Stop() should flush all pending traces")
}

func TestErrorPolicy_Evaluate(t *testing.T) {
	p := sampler.ErrorPolicy{}
	traceID, _ := model.NewTraceID()

	// No error → abstain
	spans := []*model.Span{makeSpan(traceID, false, 10*time.Millisecond)}
	keep, reason := p.Evaluate(spans, 10*time.Millisecond)
	assert.False(t, keep)
	assert.Empty(t, reason)

	// Error → keep
	spans2 := []*model.Span{makeSpan(traceID, true, 10*time.Millisecond)}
	keep2, reason2 := p.Evaluate(spans2, 10*time.Millisecond)
	assert.True(t, keep2)
	assert.Equal(t, "has_error", reason2)
}
