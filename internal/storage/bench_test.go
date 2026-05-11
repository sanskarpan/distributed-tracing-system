package storage_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/storage"
)

func BenchmarkMemoryStore_Upsert(b *testing.B) {
	benchmarkUpsert(b, storage.NewMemoryStore(100000))
}

func BenchmarkMemoryStore_Query(b *testing.B) {
	store := storage.NewMemoryStore(10000)
	populateStore(b, store, 5000)
	query := &storage.TraceQuery{Limit: 20, SortBy: "receivedAt", SortDesc: true}
	benchmarkQuery(b, store, query)
}

func BenchmarkMemoryStore_ConcurrentUpsert(b *testing.B) {
	benchmarkConcurrentUpsert(b, storage.NewMemoryStore(100000))
}

func BenchmarkMemoryStore_QueryFilters(b *testing.B) {
	store := storage.NewMemoryStore(10000)
	populateStore(b, store, 5000)
	query := &storage.TraceQuery{
		ServiceName: "svc-3",
		HasError:    boolPtr(true),
		Limit:       25,
		SortBy:      "receivedAt",
		SortDesc:    true,
	}
	benchmarkQuery(b, store, query)
}

func BenchmarkMemoryStore_QueryPagination(b *testing.B) {
	store := storage.NewMemoryStore(10000)
	populateStore(b, store, 5000)
	query := &storage.TraceQuery{
		Limit:    50,
		Offset:   500,
		SortBy:   "receivedAt",
		SortDesc: true,
	}
	benchmarkQuery(b, store, query)
}

func BenchmarkBadgerStore_Upsert(b *testing.B) {
	store := openBenchmarkBadgerStore(b)
	benchmarkUpsert(b, store)
}

func BenchmarkBadgerStore_ConcurrentUpsert(b *testing.B) {
	store := openBenchmarkBadgerStore(b)
	benchmarkConcurrentUpsert(b, store)
}

func BenchmarkBadgerStore_QueryFilters(b *testing.B) {
	store := openBenchmarkBadgerStore(b)
	populateStore(b, store, 5000)
	query := &storage.TraceQuery{
		ServiceName: "svc-3",
		HasError:    boolPtr(true),
		Limit:       25,
		SortBy:      "receivedAt",
		SortDesc:    true,
	}
	benchmarkQuery(b, store, query)
}

func BenchmarkBadgerStore_QueryPagination(b *testing.B) {
	store := openBenchmarkBadgerStore(b)
	populateStore(b, store, 5000)
	query := &storage.TraceQuery{
		Limit:    50,
		Offset:   500,
		SortBy:   "receivedAt",
		SortDesc: true,
	}
	benchmarkQuery(b, store, query)
}

func benchmarkUpsert(b *testing.B, store storage.TraceStore) {
	b.Helper()
	b.ReportAllocs()
	start := time.Unix(1_700_000_000, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trace := benchmarkTrace(b, i, start)
		require.NoError(b, store.Upsert(trace))
	}
}

func benchmarkConcurrentUpsert(b *testing.B, store storage.TraceStore) {
	b.Helper()
	b.ReportAllocs()
	start := time.Unix(1_700_000_000, 0)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			trace := benchmarkTrace(b, i, start)
			require.NoError(b, store.Upsert(trace))
			i++
		}
	})
}

func benchmarkQuery(b *testing.B, store storage.TraceStore, query *storage.TraceQuery) {
	b.Helper()
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Query(query)
		require.NoError(b, err)
	}
}

func populateStore(b *testing.B, store storage.TraceStore, count int) {
	b.Helper()
	start := time.Unix(1_700_000_000, 0)
	for i := 0; i < count; i++ {
		require.NoError(b, store.Upsert(benchmarkTrace(b, i, start)))
	}
}

func benchmarkTrace(b *testing.B, i int, start time.Time) *model.Trace {
	b.Helper()

	traceID, err := model.NewTraceID()
	require.NoError(b, err)
	rootID, err := model.NewSpanID()
	require.NoError(b, err)
	childID, err := model.NewSpanID()
	require.NoError(b, err)

	service := fmt.Sprintf("svc-%d", i%10)
	op := fmt.Sprintf("op-%d", i%5)
	startTime := start.Add(time.Duration(i) * time.Millisecond)
	endTime := startTime.Add(time.Duration(20+(i%80)) * time.Millisecond)
	hasError := i%7 == 0
	status := model.SpanStatus{}
	if hasError {
		status = model.SpanStatus{Code: model.StatusError, Message: "benchmark error"}
	}

	root := &model.Span{
		TraceID:     traceID,
		SpanID:      rootID,
		ServiceName: service,
		Name:        op,
		StartTime:   startTime,
		EndTime:     endTime,
		Status:      status,
		HasError:    hasError,
		Attributes: []model.KeyValue{
			model.StringKV("env", "bench"),
			model.StringKV("route", fmt.Sprintf("/api/%d", i%20)),
		},
	}
	child := &model.Span{
		TraceID:      traceID,
		SpanID:       childID,
		ParentSpanID: rootID,
		ServiceName:  "downstream",
		Name:         "call",
		StartTime:    startTime.Add(5 * time.Millisecond),
		EndTime:      endTime.Add(-2 * time.Millisecond),
		Attributes: []model.KeyValue{
			model.StringKV("peer.service", "downstream"),
		},
	}

	return &model.Trace{
		TraceID:    traceID,
		Spans:      []*model.Span{root, child},
		RootSpan:   root,
		Services:   []string{"downstream", service},
		SpanCount:  2,
		ErrorCount: boolToErrorCount(hasError),
		Duration:   endTime.Sub(startTime),
		ReceivedAt: endTime,
	}
}

func openBenchmarkBadgerStore(b *testing.B) *storage.BadgerStore {
	b.Helper()
	store, err := storage.OpenBadger(b.TempDir(), 100000)
	require.NoError(b, err)
	b.Cleanup(func() {
		require.NoError(b, store.Close())
	})
	return store
}

func boolPtr(v bool) *bool {
	return &v
}

func boolToErrorCount(v bool) int {
	if v {
		return 1
	}
	return 0
}
