package api

import (
	"net/http"
	"runtime"
	"sync"
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

// HandleHealthz is a liveness probe: returns 200 as long as the process is running.
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

// HandleReadyz is a readiness probe: returns 200 when the collector is ready to serve traffic.
func HandleReadyz(w http.ResponseWriter, r *http.Request) {
	mem := readMemStatsCached()
	writeJSON(w, map[string]any{
		"status":     "ready",
		"uptimeSec":  time.Since(startTime).Seconds(),
		"goroutines": runtime.NumGoroutine(),
		"heapMB":     mem.HeapAlloc / 1024 / 1024,
	})
}
