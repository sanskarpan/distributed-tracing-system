package storage

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
)

func makeTraceID(t *testing.T) model.TraceID {
	t.Helper()
	id, err := model.NewTraceID()
	require.NoError(t, err)
	return id
}

func makeSpanID(t *testing.T) model.SpanID {
	t.Helper()
	id, err := model.NewSpanID()
	require.NoError(t, err)
	return id
}

// buildTrace creates a simple trace with a single root span.
func buildTrace(traceID model.TraceID, svc, op string, dur time.Duration, hasErr bool, receivedAt time.Time) *model.Trace {
	spanID, _ := model.NewSpanID()
	now := time.Now()
	statusCode := model.StatusOK
	errCount := 0
	if hasErr {
		statusCode = model.StatusError
		errCount = 1
	}
	span := &model.Span{
		TraceID:     traceID,
		SpanID:      spanID,
		ServiceName: svc,
		Name:        op,
		StartTime:   now,
		EndTime:     now.Add(dur),
		Status:      model.SpanStatus{Code: statusCode},
	}
	return &model.Trace{
		TraceID:    traceID,
		Spans:      []*model.Span{span},
		RootSpan:   span,
		Services:   []string{svc},
		Duration:   dur,
		SpanCount:  1,
		ErrorCount: errCount,
		ReceivedAt: receivedAt,
	}
}

func TestMemoryStore_UpsertGet(t *testing.T) {
	store := NewMemoryStore(100)
	id := makeTraceID(t)
	tr := buildTrace(id, "svc-a", "op-1", 100*time.Millisecond, false, time.Now())

	err := store.Upsert(tr)
	require.NoError(t, err)

	got, ok := store.Get(id)
	require.True(t, ok)
	assert.Equal(t, id, got.TraceID)
	assert.Equal(t, 1, got.SpanCount)
}

func TestMemoryStore_GetMissing(t *testing.T) {
	store := NewMemoryStore(100)
	id := makeTraceID(t)
	_, ok := store.Get(id)
	assert.False(t, ok)
}

func TestMemoryStore_QueryByService(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	id1 := makeTraceID(t)
	id2 := makeTraceID(t)
	id3 := makeTraceID(t)

	require.NoError(t, store.Upsert(buildTrace(id1, "svc-a", "op", 10*time.Millisecond, false, now)))
	require.NoError(t, store.Upsert(buildTrace(id2, "svc-a", "op", 20*time.Millisecond, false, now.Add(time.Millisecond))))
	require.NoError(t, store.Upsert(buildTrace(id3, "svc-b", "op", 30*time.Millisecond, false, now.Add(2*time.Millisecond))))

	res, err := store.Query(&TraceQuery{ServiceName: "svc-a", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total)
	assert.Equal(t, 2, len(res.Traces))
	for _, s := range res.Traces {
		assert.Equal(t, "svc-a", s.RootService)
	}
}

func TestMemoryStore_QueryByError(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	id1 := makeTraceID(t)
	id2 := makeTraceID(t)
	id3 := makeTraceID(t)

	require.NoError(t, store.Upsert(buildTrace(id1, "svc", "op", 10*time.Millisecond, true, now)))
	require.NoError(t, store.Upsert(buildTrace(id2, "svc", "op", 20*time.Millisecond, false, now.Add(time.Millisecond))))
	require.NoError(t, store.Upsert(buildTrace(id3, "svc", "op", 30*time.Millisecond, true, now.Add(2*time.Millisecond))))

	hasErr := true
	res, err := store.Query(&TraceQuery{HasError: &hasErr, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total)
	for _, s := range res.Traces {
		assert.True(t, s.HasError)
	}

	noErr := false
	res2, err := store.Query(&TraceQuery{HasError: &noErr, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, res2.Total)
	assert.False(t, res2.Traces[0].HasError)
}

func TestMemoryStore_QueryDurationFilter(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	id1 := makeTraceID(t)
	id2 := makeTraceID(t)
	id3 := makeTraceID(t)

	require.NoError(t, store.Upsert(buildTrace(id1, "svc", "op", 10*time.Millisecond, false, now)))
	require.NoError(t, store.Upsert(buildTrace(id2, "svc", "op", 100*time.Millisecond, false, now.Add(time.Millisecond))))
	require.NoError(t, store.Upsert(buildTrace(id3, "svc", "op", 500*time.Millisecond, false, now.Add(2*time.Millisecond))))

	minDur := 50 * time.Millisecond
	maxDur := 200 * time.Millisecond
	res, err := store.Query(&TraceQuery{MinDuration: &minDur, MaxDuration: &maxDur, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	assert.Equal(t, 100*time.Millisecond, res.Traces[0].Duration)
}

func TestMemoryStore_Pagination(t *testing.T) {
	store := NewMemoryStore(100)
	base := time.Now()

	for i := 0; i < 10; i++ {
		id := makeTraceID(t)
		require.NoError(t, store.Upsert(buildTrace(id, "svc", "op", time.Duration(i+1)*10*time.Millisecond, false, base.Add(time.Duration(i)*time.Millisecond))))
	}

	// Page 1: offset=0, limit=3
	res1, err := store.Query(&TraceQuery{Limit: 3, Offset: 0, SortBy: "receivedAt"})
	require.NoError(t, err)
	assert.Equal(t, 10, res1.Total)
	assert.Equal(t, 3, len(res1.Traces))
	assert.True(t, res1.HasMore)

	// Page 2: offset=3, limit=3
	res2, err := store.Query(&TraceQuery{Limit: 3, Offset: 3, SortBy: "receivedAt"})
	require.NoError(t, err)
	assert.Equal(t, 10, res2.Total)
	assert.Equal(t, 3, len(res2.Traces))

	// Pages should not overlap (different trace IDs)
	ids1 := make(map[model.TraceID]bool)
	for _, s := range res1.Traces {
		ids1[s.TraceID] = true
	}
	for _, s := range res2.Traces {
		assert.False(t, ids1[s.TraceID], "pages should not overlap")
	}
}

func TestMemoryStore_SortByDurationDesc(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	durations := []time.Duration{50 * time.Millisecond, 200 * time.Millisecond, 10 * time.Millisecond, 500 * time.Millisecond}
	for _, d := range durations {
		id := makeTraceID(t)
		require.NoError(t, store.Upsert(buildTrace(id, "svc", "op", d, false, now)))
	}

	res, err := store.Query(&TraceQuery{SortBy: "duration", SortDesc: true, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Total)
	require.Len(t, res.Traces, 4)
	// Verify descending order
	for i := 1; i < len(res.Traces); i++ {
		assert.GreaterOrEqual(t, res.Traces[i-1].Duration, res.Traces[i].Duration)
	}
}

func TestMemoryStore_Eviction(t *testing.T) {
	maxSize := 50
	store := NewMemoryStore(maxSize)
	base := time.Now()

	total := maxSize + 100
	for i := 0; i < total; i++ {
		id := makeTraceID(t)
		tr := buildTrace(id, "svc", "op", 10*time.Millisecond, false, base.Add(time.Duration(i)*time.Millisecond))
		require.NoError(t, store.Upsert(tr))
	}

	stats := store.Stats()
	assert.Equal(t, maxSize, stats.TraceCount, "store should not exceed maxSize after eviction")
}

func TestMemoryStore_ConcurrentUpsertQuery(t *testing.T) {
	store := NewMemoryStore(1000)
	var wg sync.WaitGroup
	n := 50

	// Concurrent upserts
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := makeTraceID(t)
			tr := buildTrace(id, "svc-concurrent", "op", 10*time.Millisecond, false, time.Now())
			_ = store.Upsert(tr)
		}()
	}

	// Concurrent queries
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.Query(&TraceQuery{ServiceName: "svc-concurrent", Limit: 10})
		}()
	}

	wg.Wait()
	// No race is the test — if the race detector doesn't fire, we pass
}

func TestMemoryStore_Services(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	for _, svc := range []string{"svc-b", "svc-a", "svc-c"} {
		id := makeTraceID(t)
		require.NoError(t, store.Upsert(buildTrace(id, svc, "op", 10*time.Millisecond, false, now)))
	}

	svcs := store.Services()
	assert.Equal(t, []string{"svc-a", "svc-b", "svc-c"}, svcs)
}

func TestMemoryStore_Operations(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	for _, op := range []string{"op-2", "op-1", "op-3"} {
		id := makeTraceID(t)
		require.NoError(t, store.Upsert(buildTrace(id, "svc", op, 10*time.Millisecond, false, now)))
	}

	ops := store.Operations("svc")
	assert.Equal(t, []string{"op-1", "op-2", "op-3"}, ops)
}

func TestMemoryStore_QueryByAttributeValueTypes(t *testing.T) {
	store := NewMemoryStore(100)
	now := time.Now()

	makeTrace := func(attrs ...model.KeyValue) *model.Trace {
		id := makeTraceID(t)
		tr := buildTrace(id, "svc", "op", 10*time.Millisecond, false, now)
		tr.Spans[0].Attributes = attrs
		return tr
	}

	require.NoError(t, store.Upsert(makeTrace(model.StringKV("http.method", "GET"))))
	require.NoError(t, store.Upsert(makeTrace(model.BoolKV("cache.hit", false))))
	require.NoError(t, store.Upsert(makeTrace(model.FloatKV("retry.backoff", 12.5))))
	require.NoError(t, store.Upsert(makeTrace(model.IntKV("http.status_code", 503))))

	cases := []struct {
		name   string
		filter string
	}{
		{name: "string", filter: "http.method=get"},
		{name: "bool", filter: "cache.hit=false"},
		{name: "float", filter: "retry.backoff=12.5"},
		{name: "int", filter: "http.status_code=503"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := store.Query(&TraceQuery{AttributeKV: tc.filter, Limit: 10})
			require.NoError(t, err)
			require.Len(t, res.Traces, 1, "filter %q should match exactly one trace", tc.filter)
		})
	}
}
