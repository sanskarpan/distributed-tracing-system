package analysis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
)

func newSpan(t *testing.T, svc, name string, start, end time.Time) *model.Span {
	t.Helper()
	sid, err := model.NewSpanID()
	require.NoError(t, err)
	tid, err := model.NewTraceID()
	require.NoError(t, err)
	return &model.Span{
		TraceID:     tid,
		SpanID:      sid,
		ServiceName: svc,
		Name:        name,
		StartTime:   start,
		EndTime:     end,
	}
}

func TestCriticalPath_SingleSpan(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()
	span := newSpan(t, "svc", "op", now, now.Add(100*time.Millisecond))

	trace := &model.Trace{
		RootSpan:  span,
		Spans:     []*model.Span{span},
		SpanCount: 1,
	}

	path := a.ComputeCriticalPath(trace)
	require.Len(t, path, 1)
	assert.Equal(t, span.SpanID, path[0].SpanID)
}

func TestCriticalPath_LinearChain(t *testing.T) {
	// A→B→C, all sequential
	a := NewAnalyzer()
	now := time.Now()

	spanA := newSpan(t, "svc", "A", now, now.Add(300*time.Millisecond))
	spanB := newSpan(t, "svc", "B", now.Add(10*time.Millisecond), now.Add(200*time.Millisecond))
	spanC := newSpan(t, "svc", "C", now.Add(20*time.Millisecond), now.Add(150*time.Millisecond))

	// Link: A→B→C
	spanB.Children = []*model.Span{spanC}
	spanA.Children = []*model.Span{spanB}

	trace := &model.Trace{
		RootSpan:  spanA,
		Spans:     []*model.Span{spanA, spanB, spanC},
		SpanCount: 3,
	}

	path := a.ComputeCriticalPath(trace)
	require.Len(t, path, 3)
	assert.Equal(t, spanA.SpanID, path[0].SpanID)
	assert.Equal(t, spanB.SpanID, path[1].SpanID)
	assert.Equal(t, spanC.SpanID, path[2].SpanID)
}

func TestCriticalPath_ParallelChildren(t *testing.T) {
	// Root has two children: short (10ms) and long (500ms).
	// Critical path should go through the long one.
	a := NewAnalyzer()
	now := time.Now()

	root := newSpan(t, "svc", "root", now, now.Add(600*time.Millisecond))
	short := newSpan(t, "svc", "short", now.Add(10*time.Millisecond), now.Add(20*time.Millisecond))
	long := newSpan(t, "svc", "long", now.Add(10*time.Millisecond), now.Add(510*time.Millisecond))

	root.Children = []*model.Span{short, long}

	trace := &model.Trace{
		RootSpan:  root,
		Spans:     []*model.Span{root, short, long},
		SpanCount: 3,
	}

	path := a.ComputeCriticalPath(trace)
	require.Len(t, path, 2)
	assert.Equal(t, root.SpanID, path[0].SpanID)
	assert.Equal(t, long.SpanID, path[1].SpanID)
}

func TestCriticalPath_NilRoot(t *testing.T) {
	a := NewAnalyzer()
	trace := &model.Trace{RootSpan: nil}
	path := a.ComputeCriticalPath(trace)
	assert.Nil(t, path)
}

func TestDetectParallelGroups(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	root := newSpan(t, "svc", "root", now, now.Add(500*time.Millisecond))
	// child1 and child2 overlap
	child1 := newSpan(t, "svc", "c1", now.Add(10*time.Millisecond), now.Add(200*time.Millisecond))
	child2 := newSpan(t, "svc", "c2", now.Add(50*time.Millisecond), now.Add(300*time.Millisecond))
	// child3 is sequential after child2
	child3 := newSpan(t, "svc", "c3", now.Add(350*time.Millisecond), now.Add(450*time.Millisecond))

	root.Children = []*model.Span{child1, child2, child3}

	groups := a.DetectParallelGroups(root)
	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Spans, 2)
}

func TestDependencyGraph(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	// Build 10 traces. Each trace has: frontend (root) → api-gateway (client) → payment-svc (server)
	traces := make([]*model.Trace, 10)
	for i := range traces {
		tid, err := model.NewTraceID()
		require.NoError(t, err)

		makeID := func() model.SpanID {
			id, _ := model.NewSpanID()
			return id
		}

		rootID := makeID()
		clientID := makeID()
		serverID := makeID()

		root := &model.Span{
			TraceID:     tid,
			SpanID:      rootID,
			ServiceName: "frontend",
			Name:        "GET /checkout",
			Kind:        model.SpanKindServer,
			StartTime:   now,
			EndTime:     now.Add(300 * time.Millisecond),
		}
		client := &model.Span{
			TraceID:      tid,
			SpanID:       clientID,
			ParentSpanID: rootID,
			ServiceName:  "api-gateway",
			Name:         "POST payment",
			Kind:         model.SpanKindClient,
			StartTime:    now.Add(10 * time.Millisecond),
			EndTime:      now.Add(280 * time.Millisecond),
			Attributes:   []model.KeyValue{model.StringKV("peer.service", "payment-svc")},
		}
		// Mark 2 of 10 traces as errors on the client span
		if i < 2 {
			client.Status = model.SpanStatus{Code: model.StatusError}
		}
		server := &model.Span{
			TraceID:      tid,
			SpanID:       serverID,
			ParentSpanID: clientID,
			ServiceName:  "payment-svc",
			Name:         "charge",
			Kind:         model.SpanKindServer,
			StartTime:    now.Add(20 * time.Millisecond),
			EndTime:      now.Add(270 * time.Millisecond),
		}
		traces[i] = &model.Trace{
			TraceID:   tid,
			Spans:     []*model.Span{root, client, server},
			SpanCount: 3,
		}
	}

	graph := a.BuildDependencyGraph(traces, now.Add(-time.Second))

	// Should have 3 services
	assert.Len(t, graph.Services, 3)

	// Should have 1 edge: api-gateway → payment-svc
	require.Len(t, graph.Edges, 1)
	edge := graph.Edges[0]
	assert.Equal(t, "api-gateway", edge.Caller)
	assert.Equal(t, "payment-svc", edge.Callee)
	assert.Equal(t, int64(10), edge.Count)
	assert.Equal(t, int64(2), edge.ErrorCount)
}

func TestCompareTraces_SameTrace(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	span := newSpan(t, "svc", "op", now, now.Add(100*time.Millisecond))
	trace := &model.Trace{
		TraceID:   span.TraceID,
		Spans:     []*model.Span{span},
		SpanCount: 1,
		Duration:  100 * time.Millisecond,
	}

	result := a.CompareTraces(trace, trace)

	assert.Equal(t, 0.0, result.DurationDeltaMs)
	assert.Equal(t, 0, result.SpanCountDelta)
	assert.Len(t, result.OnlyInBase, 0)
	assert.Len(t, result.OnlyInCompare, 0)
	assert.Len(t, result.Matched, 1)
}

func TestCompareTraces_AddedSpan(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	// Base trace: 1 span
	baseSpan := newSpan(t, "svc", "op", now, now.Add(100*time.Millisecond))
	base := &model.Trace{
		TraceID:   baseSpan.TraceID,
		Spans:     []*model.Span{baseSpan},
		SpanCount: 1,
		Duration:  100 * time.Millisecond,
	}

	// Compare trace: same span + extra span
	cmpSpan := newSpan(t, "svc", "op", now, now.Add(100*time.Millisecond))
	extraSpan := newSpan(t, "svc", "extra-op", now.Add(10*time.Millisecond), now.Add(50*time.Millisecond))
	cmp := &model.Trace{
		TraceID:   cmpSpan.TraceID,
		Spans:     []*model.Span{cmpSpan, extraSpan},
		SpanCount: 2,
		Duration:  100 * time.Millisecond,
	}

	result := a.CompareTraces(base, cmp)

	assert.Equal(t, 1, result.SpanCountDelta)
	assert.Len(t, result.Matched, 1)
	require.Len(t, result.OnlyInCompare, 1)
	assert.Equal(t, extraSpan.SpanID, result.OnlyInCompare[0])
	assert.Len(t, result.OnlyInBase, 0)
}

func TestCompareTraces_MatchesRepeatedOperationsByParentPath(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	baseRoot := newSpan(t, "api", "root", now, now.Add(300*time.Millisecond))
	baseCheckout := newSpan(t, "api", "checkout", now.Add(10*time.Millisecond), now.Add(120*time.Millisecond))
	baseShipping := newSpan(t, "api", "shipping", now.Add(140*time.Millisecond), now.Add(260*time.Millisecond))
	baseCheckout.ParentSpanID = baseRoot.SpanID
	baseShipping.ParentSpanID = baseRoot.SpanID

	baseCheckoutDB := newSpan(t, "db", "query", now.Add(20*time.Millisecond), now.Add(60*time.Millisecond))
	baseShippingDB := newSpan(t, "db", "query", now.Add(150*time.Millisecond), now.Add(230*time.Millisecond))
	baseCheckoutDB.ParentSpanID = baseCheckout.SpanID
	baseShippingDB.ParentSpanID = baseShipping.SpanID

	base := &model.Trace{
		TraceID:   baseRoot.TraceID,
		Spans:     []*model.Span{baseRoot, baseCheckout, baseShipping, baseCheckoutDB, baseShippingDB},
		SpanCount: 5,
		Duration:  300 * time.Millisecond,
	}

	cmpRoot := newSpan(t, "api", "root", now, now.Add(320*time.Millisecond))
	cmpCheckout := newSpan(t, "api", "checkout", now.Add(10*time.Millisecond), now.Add(140*time.Millisecond))
	cmpShipping := newSpan(t, "api", "shipping", now.Add(140*time.Millisecond), now.Add(280*time.Millisecond))
	cmpCheckout.ParentSpanID = cmpRoot.SpanID
	cmpShipping.ParentSpanID = cmpRoot.SpanID

	cmpCheckoutDB := newSpan(t, "db", "query", now.Add(20*time.Millisecond), now.Add(90*time.Millisecond))
	cmpShippingDB := newSpan(t, "db", "query", now.Add(150*time.Millisecond), now.Add(210*time.Millisecond))
	cmpCheckoutDB.ParentSpanID = cmpCheckout.SpanID
	cmpShippingDB.ParentSpanID = cmpShipping.SpanID

	cmp := &model.Trace{
		TraceID:   cmpRoot.TraceID,
		Spans:     []*model.Span{cmpRoot, cmpShippingDB, cmpCheckoutDB, cmpShipping, cmpCheckout},
		SpanCount: 5,
		Duration:  320 * time.Millisecond,
	}

	result := a.CompareTraces(base, cmp)

	require.Len(t, result.Matched, 5)
	assert.Contains(t, result.Matched, SpanDiff{
		BaseSpanID:      baseCheckoutDB.SpanID,
		CompareSpanID:   cmpCheckoutDB.SpanID,
		DurationDeltaMs: 30,
	})
	assert.Contains(t, result.Matched, SpanDiff{
		BaseSpanID:      baseShippingDB.SpanID,
		CompareSpanID:   cmpShippingDB.SpanID,
		DurationDeltaMs: -20,
	})
	assert.Empty(t, result.OnlyInBase)
	assert.Empty(t, result.OnlyInCompare)
}

func TestCompareTraces_MatchesRepeatedSiblingsByStartTime(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	baseRoot := newSpan(t, "api", "root", now, now.Add(200*time.Millisecond))
	baseRetry1 := newSpan(t, "worker", "retry", now.Add(10*time.Millisecond), now.Add(30*time.Millisecond))
	baseRetry2 := newSpan(t, "worker", "retry", now.Add(40*time.Millisecond), now.Add(70*time.Millisecond))
	baseRetry1.ParentSpanID = baseRoot.SpanID
	baseRetry2.ParentSpanID = baseRoot.SpanID

	base := &model.Trace{
		TraceID:   baseRoot.TraceID,
		Spans:     []*model.Span{baseRoot, baseRetry1, baseRetry2},
		SpanCount: 3,
		Duration:  200 * time.Millisecond,
	}

	cmpRoot := newSpan(t, "api", "root", now, now.Add(210*time.Millisecond))
	cmpRetry1 := newSpan(t, "worker", "retry", now.Add(10*time.Millisecond), now.Add(35*time.Millisecond))
	cmpRetry2 := newSpan(t, "worker", "retry", now.Add(40*time.Millisecond), now.Add(90*time.Millisecond))
	cmpRetry1.ParentSpanID = cmpRoot.SpanID
	cmpRetry2.ParentSpanID = cmpRoot.SpanID

	cmp := &model.Trace{
		TraceID:   cmpRoot.TraceID,
		Spans:     []*model.Span{cmpRoot, cmpRetry2, cmpRetry1},
		SpanCount: 3,
		Duration:  210 * time.Millisecond,
	}

	result := a.CompareTraces(base, cmp)

	require.Len(t, result.Matched, 3)
	assert.Contains(t, result.Matched, SpanDiff{
		BaseSpanID:      baseRetry1.SpanID,
		CompareSpanID:   cmpRetry1.SpanID,
		DurationDeltaMs: 5,
	})
	assert.Contains(t, result.Matched, SpanDiff{
		BaseSpanID:      baseRetry2.SpanID,
		CompareSpanID:   cmpRetry2.SpanID,
		DurationDeltaMs: 20,
	})
	assert.Empty(t, result.OnlyInBase)
	assert.Empty(t, result.OnlyInCompare)
}

func TestDetectGaps(t *testing.T) {
	a := NewAnalyzer()
	now := time.Now()

	root := newSpan(t, "svc", "root", now, now.Add(500*time.Millisecond))
	child1 := newSpan(t, "svc", "c1", now.Add(10*time.Millisecond), now.Add(100*time.Millisecond))
	// Gap between 100ms and 200ms
	child2 := newSpan(t, "svc", "c2", now.Add(200*time.Millisecond), now.Add(300*time.Millisecond))

	root.Children = []*model.Span{child1, child2}

	trace := &model.Trace{RootSpan: root}
	gaps := a.DetectGaps(trace)
	require.Len(t, gaps, 1)
	assert.Equal(t, 100*time.Millisecond, gaps[0].Duration)
	assert.Equal(t, child1.SpanID, gaps[0].Before.SpanID)
	assert.Equal(t, child2.SpanID, gaps[0].After.SpanID)
}
