package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

type sequenceSampler struct {
	mu        sync.Mutex
	decisions []sampler.SamplingDecision
	calls     int
	seen      []sampler.SamplingParameters
}

func (s *sequenceSampler) ShouldSample(p sampler.SamplingParameters) sampler.SamplingResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seen = append(s.seen, p)
	decision := s.decisions[len(s.seen)-1]
	s.calls++
	return sampler.SamplingResult{Decision: decision}
}

func (s *sequenceSampler) Name() string { return "sequence" }
func (s *sequenceSampler) Config() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"calls": s.calls}
}

func TestPipelineShutdownFlushesPendingAssemblerTrace(t *testing.T) {
	store := storage.NewMemoryStore(100)
	pipeline := NewPipeline(
		store,
		metrics.NewMetricsStore(),
		NewSSEBus(),
		sampler.NewAlways(),
		analysis.NewAnalyzer(),
		10*time.Second,
	)

	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)
	now := time.Now()

	accepted, dropped, err := pipeline.IngestSpans([]*model.Span{{
		TraceID:     traceID,
		SpanID:      spanID,
		ServiceName: "svc",
		Name:        "op",
		StartTime:   now,
		EndTime:     now.Add(50 * time.Millisecond),
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, 0, dropped)

	_, found := store.Get(traceID)
	assert.False(t, found, "trace should still be pending before shutdown flush")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, pipeline.Shutdown(ctx))

	trace, found := store.Get(traceID)
	require.True(t, found, "shutdown should flush the pending trace into storage")
	require.NotNil(t, trace)
	assert.Equal(t, 1, trace.SpanCount)
}

func TestPipelineShutdownFlushesTailSamplerBuffer(t *testing.T) {
	store := storage.NewMemoryStore(100)
	pipeline := NewPipeline(
		store,
		metrics.NewMetricsStore(),
		NewSSEBus(),
		sampler.NewAlways(),
		analysis.NewAnalyzer(),
		10*time.Second,
	)

	tailSampler := sampler.NewTailSampler(
		10*time.Second,
		100,
		[]sampler.TailPolicy{sampler.ErrorPolicy{}},
		func(spans []*model.Span) {
			for _, span := range spans {
				pipeline.processSpan(span)
			}
		},
		func(model.TraceID) {},
	)
	pipeline.SwapSampler(tailSampler)

	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)
	now := time.Now()

	accepted, dropped, err := pipeline.IngestSpans([]*model.Span{{
		TraceID:     traceID,
		SpanID:      spanID,
		ServiceName: "svc",
		Name:        "op",
		StartTime:   now,
		EndTime:     now.Add(50 * time.Millisecond),
		Status:      model.SpanStatus{Code: model.StatusError, Message: "boom"},
		HasError:    true,
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, 0, dropped)
	assert.Equal(t, 1, tailSampler.BufferedCount())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, pipeline.Shutdown(ctx))

	trace, found := store.Get(traceID)
	require.True(t, found, "shutdown should flush accepted tail-sampled traces into storage")
	require.NotNil(t, trace)
	assert.Equal(t, 1, trace.SpanCount)
	assert.Equal(t, 0, tailSampler.BufferedCount())
}

func TestPipeline_HeadSamplingCachesDecisionPerTrace(t *testing.T) {
	store := storage.NewMemoryStore(100)
	samplerSeq := &sequenceSampler{
		decisions: []sampler.SamplingDecision{sampler.Sample, sampler.Drop},
	}
	pipeline := NewPipeline(
		store,
		metrics.NewMetricsStore(),
		NewSSEBus(),
		samplerSeq,
		analysis.NewAnalyzer(),
		50*time.Millisecond,
	)

	traceA, err := model.NewTraceID()
	require.NoError(t, err)
	traceB, err := model.NewTraceID()
	require.NoError(t, err)
	rootA, err := model.NewSpanID()
	require.NoError(t, err)
	childA, err := model.NewSpanID()
	require.NoError(t, err)
	rootB, err := model.NewSpanID()
	require.NoError(t, err)
	childB, err := model.NewSpanID()
	require.NoError(t, err)
	now := time.Now()

	accepted, dropped, err := pipeline.IngestSpans([]*model.Span{
		{TraceID: traceA, SpanID: childA, ParentSpanID: rootA, ServiceName: "svc-a", Name: "child", StartTime: now.Add(5 * time.Millisecond), EndTime: now.Add(15 * time.Millisecond)},
		{TraceID: traceB, SpanID: childB, ParentSpanID: rootB, ServiceName: "svc-b", Name: "child", StartTime: now.Add(7 * time.Millisecond), EndTime: now.Add(17 * time.Millisecond)},
		{TraceID: traceA, SpanID: rootA, ServiceName: "svc-a", Name: "root", StartTime: now, EndTime: now.Add(20 * time.Millisecond)},
		{TraceID: traceB, SpanID: rootB, ServiceName: "svc-b", Name: "root", StartTime: now, EndTime: now.Add(20 * time.Millisecond)},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, accepted)
	assert.Equal(t, 2, dropped)

	time.Sleep(120 * time.Millisecond)

	trace, found := store.Get(traceA)
	require.True(t, found, "sampled trace should be stored")
	require.NotNil(t, trace)
	assert.Equal(t, 2, trace.SpanCount)

	_, found = store.Get(traceB)
	assert.False(t, found, "dropped trace should never be stored")

	samplerSeq.mu.Lock()
	defer samplerSeq.mu.Unlock()
	assert.Equal(t, 2, samplerSeq.calls, "sampling should be computed once per trace")
}

func TestPipeline_HeadSamplingUsesRootSpanWhenPresentInBatch(t *testing.T) {
	store := storage.NewMemoryStore(100)
	headSampler := sampler.NewRuleBased([]sampler.Rule{
		{
			ServiceName: "frontend",
			Decision:    sampler.Sample,
			Priority:    10,
		},
	}, sampler.NewNever())
	pipeline := NewPipeline(
		store,
		metrics.NewMetricsStore(),
		NewSSEBus(),
		headSampler,
		analysis.NewAnalyzer(),
		50*time.Millisecond,
	)

	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	rootID, err := model.NewSpanID()
	require.NoError(t, err)
	childID, err := model.NewSpanID()
	require.NoError(t, err)
	now := time.Now()

	accepted, dropped, err := pipeline.IngestSpans([]*model.Span{
		{TraceID: traceID, SpanID: childID, ParentSpanID: rootID, ServiceName: "db", Name: "query", StartTime: now.Add(10 * time.Millisecond), EndTime: now.Add(20 * time.Millisecond)},
		{TraceID: traceID, SpanID: rootID, ServiceName: "frontend", Name: "GET /checkout", StartTime: now, EndTime: now.Add(30 * time.Millisecond)},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, accepted)
	assert.Equal(t, 0, dropped)

	time.Sleep(120 * time.Millisecond)

	trace, found := store.Get(traceID)
	require.True(t, found, "trace should be sampled using the root span when it is present in the batch")
	require.NotNil(t, trace)
	assert.Equal(t, 2, trace.SpanCount)
}
