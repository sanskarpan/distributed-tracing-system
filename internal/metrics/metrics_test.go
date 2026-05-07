package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourname/tracing/internal/model"
)

func makeSpan(svc, op string, dur time.Duration, hasErr bool) *model.Span {
	now := time.Now()
	statusCode := model.StatusOK
	if hasErr {
		statusCode = model.StatusError
	}
	return &model.Span{
		ServiceName: svc,
		Name:        op,
		StartTime:   now,
		EndTime:     now.Add(dur),
		Status:      model.SpanStatus{Code: statusCode},
	}
}

func TestMetricsStore_RecordAndSnapshot(t *testing.T) {
	store := NewMetricsStore()

	// Record 100 spans for the same key
	for i := 0; i < 100; i++ {
		store.Record(makeSpan("svc-a", "op-1", 10*time.Millisecond, false))
	}

	snapshots := store.Snapshot()
	assert.Len(t, snapshots, 1)
	snap := snapshots[0]
	assert.Equal(t, "svc-a", snap.Service)
	assert.Equal(t, "op-1", snap.Operation)
	// Rate should be > 0 (all 100 spans in the same second bucket)
	assert.Greater(t, snap.Rate, 0.0)
	assert.Equal(t, 0.0, snap.ErrorRate)
}

func TestMetricsStore_P99KnownDistribution(t *testing.T) {
	store := NewMetricsStore()

	// Record spans with durations 1ms through 100ms
	for i := 1; i <= 100; i++ {
		store.Record(makeSpan("svc", "op", time.Duration(i)*time.Millisecond, false))
	}

	snaps := store.Snapshot()
	assert.Len(t, snaps, 1)
	p99 := snaps[0].P99Ms
	// P99 of [1..100] should be approximately 99ms ± 2ms
	assert.InDelta(t, 99.0, p99, 2.0, "P99 should be approximately 99ms")
}

func TestMetricsStore_ConcurrentRecord(t *testing.T) {
	store := NewMetricsStore()
	var wg sync.WaitGroup
	n := 50

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hasErr := idx%5 == 0
			store.Record(makeSpan("svc", "op", time.Duration(idx+1)*time.Millisecond, hasErr))
		}(i)
	}
	wg.Wait()

	snaps := store.Snapshot()
	assert.Len(t, snaps, 1)
	// Rate > 0 means spans were recorded
	assert.Greater(t, snaps[0].Rate, 0.0)
}

func TestMetricsStore_MultipleKeys(t *testing.T) {
	store := NewMetricsStore()

	store.Record(makeSpan("svc-a", "op-1", 10*time.Millisecond, false))
	store.Record(makeSpan("svc-a", "op-2", 20*time.Millisecond, true))
	store.Record(makeSpan("svc-b", "op-1", 30*time.Millisecond, false))

	snaps := store.Snapshot()
	assert.Len(t, snaps, 3)

	// Find the error one
	var errSnap *MetricSnapshot
	for i := range snaps {
		if snaps[i].Service == "svc-a" && snaps[i].Operation == "op-2" {
			errSnap = &snaps[i]
		}
	}
	assert.NotNil(t, errSnap)
	assert.Greater(t, errSnap.ErrorRate, 0.0)
}

func TestHistogram_Percentiles(t *testing.T) {
	h := NewHistogram(1000)
	for i := 1; i <= 100; i++ {
		h.Record(float64(i))
	}
	// P50 ≈ 50, P95 ≈ 95, P99 ≈ 99
	assert.InDelta(t, 50.0, h.P50(), 2.0)
	assert.InDelta(t, 95.0, h.P95(), 2.0)
	assert.InDelta(t, 99.0, h.P99(), 2.0)
}

func TestHistogram_Empty(t *testing.T) {
	h := NewHistogram(100)
	assert.Equal(t, 0.0, h.P50())
	assert.Equal(t, 0.0, h.P99())
}

func TestSlidingWindow_Rate(t *testing.T) {
	w := &SlidingWindow{}
	w.Add(60)
	rate := w.Rate()
	// 60 events in the window / 60 buckets = 1.0 per second
	assert.InDelta(t, 1.0, rate, 0.1)
}
