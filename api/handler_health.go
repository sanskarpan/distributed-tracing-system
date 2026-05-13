package api

import (
	"net/http"
	"os"
	"strconv"
	"runtime"
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
		w.WriteHeader(http.StatusServiceUnavailable)
	} else if p.maxQueueUsagePercent > 0 && queueCapacity > 0 && queueUsage >= p.maxQueueUsagePercent {
		status = "overloaded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, map[string]any{
		"status":         status,
		"uptimeSec":      time.Since(startTime).Seconds(),
		"goroutines":     runtime.NumGoroutine(),
		"heapMB":         mem.HeapAlloc / 1024 / 1024,
		"queueDepth":     queueDepth,
		"queueCapacity":  queueCapacity,
		"queueUsagePct":  queueUsage,
		"queueThreshold": p.maxQueueUsagePercent,
	})
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
