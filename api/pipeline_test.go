package api

import (
	"context"
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
