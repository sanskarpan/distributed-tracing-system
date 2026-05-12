package api

import (
	"net/http"
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
	draining atomic.Bool
}

func NewProbeState() *ProbeState {
	return &ProbeState{}
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
	if p.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, map[string]any{
		"status":     map[bool]string{true: "draining", false: "ready"}[p.draining.Load()],
		"uptimeSec":  time.Since(startTime).Seconds(),
		"goroutines": runtime.NumGoroutine(),
		"heapMB":     mem.HeapAlloc / 1024 / 1024,
	})
}
