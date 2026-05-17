package api

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var startTime = time.Now()

// memStatsCache caches runtime.MemStats to avoid triggering a STW GC pause on every probe.
var (
	memStatsMu      sync.Mutex
	cachedMemStats  runtime.MemStats
	memStatsRefresh time.Time
)

func readMemStatsCached() runtime.MemStats {
	memStatsMu.Lock()
	defer memStatsMu.Unlock()
	if time.Since(memStatsRefresh) > 10*time.Second {
		runtime.ReadMemStats(&cachedMemStats)
		memStatsRefresh = time.Now()
	}
	return cachedMemStats
}

type ProbeState struct {
	draining             atomic.Bool
	queueDepth           func() int
	queueCapacity        int
	maxQueueUsagePercent float64
}

type ProbeSnapshot struct {
	Status         string  `json:"status"`
	UptimeSec      float64 `json:"uptimeSec"`
	Goroutines     int     `json:"goroutines"`
	HeapMB         uint64  `json:"heapMB"`
	QueueDepth     int     `json:"queueDepth"`
	QueueCapacity  int     `json:"queueCapacity"`
	QueueUsagePct  float64 `json:"queueUsagePct"`
	QueueThreshold float64 `json:"queueThreshold"`
}

func NewProbeState(queueDepth func() int, queueCapacity int) *ProbeState {
	return &ProbeState{
		queueDepth:           queueDepth,
		queueCapacity:        queueCapacity,
		maxQueueUsagePercent: envFloat("READINESS_MAX_QUEUE_USAGE_PCT", 0),
	}
}

func (p *ProbeState) MarkDraining() {
	p.draining.Store(true)
}

func (p *ProbeState) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if p.draining.Load() {
		status = "draining"
	}
	writeJSON(w, map[string]string{"status": status})
}

// HandleReadyz is a readiness probe: returns 200 when the collector is ready to serve traffic.
func (p *ProbeState) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	snapshot := p.Snapshot()
	if snapshot.Status == "draining" || snapshot.Status == "overloaded" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, snapshot)
}

func (p *ProbeState) Snapshot() ProbeSnapshot {
	mem := readMemStatsCached()
	status := "ready"
	queueDepth := 0
	queueCapacity := p.queueCapacity
	queueUsage := 0.0
	if p.queueDepth != nil {
		queueDepth = p.queueDepth()
	}
	if queueCapacity > 0 {
		queueUsage = (float64(queueDepth) / float64(queueCapacity)) * 100
	}

	if p.draining.Load() {
		status = "draining"
	} else if p.maxQueueUsagePercent > 0 && queueCapacity > 0 && queueUsage >= p.maxQueueUsagePercent {
		status = "overloaded"
	}
	return ProbeSnapshot{
		Status:         status,
		UptimeSec:      time.Since(startTime).Seconds(),
		Goroutines:     runtime.NumGoroutine(),
		HeapMB:         mem.HeapAlloc / 1024 / 1024,
		QueueDepth:     queueDepth,
		QueueCapacity:  queueCapacity,
		QueueUsagePct:  queueUsage,
		QueueThreshold: p.maxQueueUsagePercent,
	}
}

func envFloat(key string, fallback float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
