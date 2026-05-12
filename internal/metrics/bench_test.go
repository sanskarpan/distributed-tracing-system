package metrics_test

import (
	"testing"
	"time"

	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/model"
)

func BenchmarkMetricsStore_Record(b *testing.B) {
	store := metrics.NewMetricsStore()
	span := &model.Span{
		ServiceName: "bench-service",
		Name:        "bench-op",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(10 * time.Millisecond),
		Status:      model.SpanStatus{Code: model.StatusOK},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Record(span)
	}
}

func BenchmarkMetricsStore_Record_Parallel(b *testing.B) {
	store := metrics.NewMetricsStore()
	span := &model.Span{
		ServiceName: "bench-service",
		Name:        "bench-op",
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(10 * time.Millisecond),
		Status:      model.SpanStatus{Code: model.StatusOK},
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			store.Record(span)
		}
	})
}

func BenchmarkMetricsStore_Snapshot(b *testing.B) {
	store := metrics.NewMetricsStore()
	// Populate with 100 service/op pairs
	for i := 0; i < 100; i++ {
		for j := 0; j < 5; j++ {
			span := &model.Span{
				ServiceName: "svc",
				Name:        "op",
				StartTime:   time.Now(),
				EndTime:     time.Now().Add(time.Duration(i+1) * time.Millisecond),
			}
			store.Record(span)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Snapshot()
	}
}
