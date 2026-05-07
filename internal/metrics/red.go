package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// MetricKey identifies a unique (service, operation) pair.
type MetricKey struct {
	Service   string
	Operation string
}

// ServiceMetrics holds rate, error, and duration metrics for a single service/operation pair.
type ServiceMetrics struct {
	Key       MetricKey
	Rate      *SlidingWindow
	Errors    *SlidingWindow
	Durations *Histogram
}

// MetricSnapshot is a point-in-time snapshot of metrics for a (service, operation) pair.
type MetricSnapshot struct {
	Service   string  `json:"service"`
	Operation string  `json:"operation"`
	Rate      float64 `json:"rate"`
	ErrorRate float64 `json:"errorRate"`
	P50Ms     float64 `json:"p50Ms"`
	P95Ms     float64 `json:"p95Ms"`
	P99Ms     float64 `json:"p99Ms"`
}

// HeatmapBucket holds aggregated span and error counts for a time bucket.
type HeatmapBucket struct {
	Ts     int64
	Spans  int64
	Errors int64
}

// MetricsStore stores RED metrics (Rate, Errors, Duration) per service/operation.
type MetricsStore struct {
	mu   sync.RWMutex
	data map[MetricKey]*ServiceMetrics
}

// NewMetricsStore creates a new MetricsStore.
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{data: make(map[MetricKey]*ServiceMetrics)}
}

// Record updates metrics for a span.
func (m *MetricsStore) Record(span *model.Span) {
	key := MetricKey{Service: span.ServiceName, Operation: span.Name}

	m.mu.RLock()
	sm, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		m.mu.Lock()
		// Double-check after acquiring write lock
		sm, ok = m.data[key]
		if !ok {
			sm = &ServiceMetrics{
				Key:       key,
				Rate:      &SlidingWindow{},
				Errors:    &SlidingWindow{},
				Durations: NewHistogram(1024),
			}
			m.data[key] = sm
		}
		m.mu.Unlock()
	}

	sm.Rate.Add(1)
	if span.Status.Code == model.StatusError {
		sm.Errors.Add(1)
	}
	durMs := float64(span.Duration()) / float64(time.Millisecond)
	if durMs < 0 {
		durMs = 0
	}
	sm.Durations.Record(durMs)
}

// HeatmapData returns 10-second aggregated span and error counts for a service over the last 60s.
// It sums across all operations for the given service (or all services if service == "").
func (m *MetricsStore) HeatmapData(service string) []HeatmapBucket {
	m.mu.RLock()
	var matching []*ServiceMetrics
	for _, sm := range m.data {
		if service == "" || sm.Key.Service == service {
			matching = append(matching, sm)
		}
	}
	m.mu.RUnlock()

	if len(matching) == 0 {
		return []HeatmapBucket{}
	}

	// Aggregate into 10s buckets (6 buckets over 60s)
	const bucketSizeS = 10
	const numBuckets = windowBuckets / bucketSizeS

	var spanTotals [numBuckets]int64
	var errTotals [numBuckets]int64
	var lastSec int64

	for _, sm := range matching {
		rateBuckets, ls := sm.Rate.RawBuckets()
		errBuckets, _ := sm.Errors.RawBuckets()
		if lastSec == 0 {
			lastSec = ls
		}
		// Aggregate 1s buckets into 10s groups starting from oldest
		for i := 0; i < windowBuckets; i++ {
			// Map raw circular index to chronological index
			sec := ls - int64(windowBuckets-1-i)
			idx := int(sec % windowBuckets)
			if idx < 0 {
				idx += windowBuckets
			}
			group := i / bucketSizeS
			spanTotals[group] += rateBuckets[idx]
			errTotals[group] += errBuckets[idx]
		}
	}

	buckets := make([]HeatmapBucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		ts := lastSec - int64((numBuckets-1-i)*bucketSizeS)
		buckets[i] = HeatmapBucket{Ts: ts, Spans: spanTotals[i], Errors: errTotals[i]}
	}
	return buckets
}

// Snapshot returns current metrics for all (service, operation) keys.
func (m *MetricsStore) Snapshot() []MetricSnapshot {
	m.mu.RLock()
	keys := make([]MetricKey, 0, len(m.data))
	sms := make([]*ServiceMetrics, 0, len(m.data))
	for k, sm := range m.data {
		keys = append(keys, k)
		sms = append(sms, sm)
	}
	m.mu.RUnlock()

	snapshots := make([]MetricSnapshot, len(keys))
	for i, sm := range sms {
		rate := sm.Rate.Rate()
		errRate := sm.Errors.Rate()
		var errorRate float64
		if rate > 0 {
			errorRate = errRate / rate
		}
		snapshots[i] = MetricSnapshot{
			Service:   sm.Key.Service,
			Operation: sm.Key.Operation,
			Rate:      rate,
			ErrorRate: errorRate,
			P50Ms:     sm.Durations.P50(),
			P95Ms:     sm.Durations.P95(),
			P99Ms:     sm.Durations.P99(),
		}
	}

	// Sort for deterministic output
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Service != snapshots[j].Service {
			return snapshots[i].Service < snapshots[j].Service
		}
		return snapshots[i].Operation < snapshots[j].Operation
	})

	return snapshots
}
