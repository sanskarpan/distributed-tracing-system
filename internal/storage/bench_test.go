package storage_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/storage"
)

func BenchmarkMemoryStore_Upsert(b *testing.B) {
	store := storage.NewMemoryStore(100000)
	spans := makeSpans(b, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr := &model.Trace{
			TraceID:    spans[0].TraceID,
			Spans:      spans,
			SpanCount:  1,
			ReceivedAt: time.Now(),
		}
		store.Upsert(tr)
	}
}

func BenchmarkMemoryStore_Query(b *testing.B) {
	store := storage.NewMemoryStore(10000)
	// Pre-populate with 5000 traces
	for i := 0; i < 5000; i++ {
		id, _ := model.NewTraceID()
		sid, _ := model.NewSpanID()
		tr := &model.Trace{
			TraceID:   id,
			Services:  []string{fmt.Sprintf("svc%d", i%10)},
			SpanCount: 1,
			ReceivedAt: time.Now(),
			Spans: []*model.Span{{
				TraceID:     id,
				SpanID:      sid,
				ServiceName: fmt.Sprintf("svc%d", i%10),
				Name:        "op",
				StartTime:   time.Now(),
				EndTime:     time.Now().Add(time.Millisecond),
			}},
		}
		store.Upsert(tr)
	}

	q := &storage.TraceQuery{Limit: 20, SortBy: "receivedAt", SortDesc: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Query(q)
	}
}

func BenchmarkMemoryStore_ConcurrentUpsert(b *testing.B) {
	store := storage.NewMemoryStore(100000)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id, _ := model.NewTraceID()
			sid, _ := model.NewSpanID()
			tr := &model.Trace{
				TraceID:    id,
				SpanCount:  1,
				ReceivedAt: time.Now(),
				Spans: []*model.Span{{
					TraceID:   id,
					SpanID:    sid,
					StartTime: time.Now(),
					EndTime:   time.Now().Add(time.Millisecond),
				}},
			}
			store.Upsert(tr)
		}
	})
}

func makeSpans(b *testing.B, n int) []*model.Span {
	b.Helper()
	spans := make([]*model.Span, n)
	traceID, _ := model.NewTraceID()
	for i := range spans {
		sid, _ := model.NewSpanID()
		spans[i] = &model.Span{
			TraceID:     traceID,
			SpanID:      sid,
			ServiceName: "bench-service",
			Name:        "bench-op",
			StartTime:   time.Now(),
			EndTime:     time.Now().Add(time.Millisecond),
		}
	}
	return spans
}
