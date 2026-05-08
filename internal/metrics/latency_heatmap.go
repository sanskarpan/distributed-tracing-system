package metrics

import (
	"sync"
	"time"
)

// latencyBounds are the upper bounds (in ms) for latency buckets.
// Spans with duration > the last bound go into the overflow bucket.
var latencyBounds = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

const heatmapWindowSec  = 10 // seconds per time bucket
const heatmapNumWindows = 6  // 6 × 10s = 60s of history
const heatmapNumBands   = 11 // len(latencyBounds) + 1 overflow; kept as constant for arrays

// LatencyHeatmapBucket holds span counts per latency band for one time window.
type LatencyHeatmapBucket struct {
	Ts     int64   `json:"ts"`     // unix seconds (start of the 10s window)
	Counts []int64 `json:"counts"` // count per latency band (len = len(latencyBounds)+1)
}

// LatencyHeatmapData is the full 2D heatmap (time × latency).
type LatencyHeatmapData struct {
	Bounds  []float64              `json:"bounds"`  // upper bound of each latency band in ms
	Buckets []LatencyHeatmapBucket `json:"buckets"` // time-ordered, oldest first
}

// serviceLatencyHeatmap tracks latency distributions across rolling 10-second windows.
type serviceLatencyHeatmap struct {
	mu      sync.Mutex
	windows [heatmapNumWindows][heatmapNumBands]int64 // ring buffer of count arrays
	tsFor   [heatmapNumWindows]int64                  // unix 10s epoch for each slot
	head    int                                       // index of the most-recent slot
}

func (h *serviceLatencyHeatmap) record(durationMs float64) {
	slot := time.Now().Unix() / heatmapWindowSec
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.tsFor[h.head] != slot {
		// Advance to a new slot
		next := (h.head + 1) % heatmapNumWindows
		h.windows[next] = [heatmapNumBands]int64{}
		h.tsFor[next] = slot
		h.head = next
	}

	// Find the bucket index
	idx := len(latencyBounds) // overflow bucket
	for i, bound := range latencyBounds {
		if durationMs <= bound {
			idx = i
			break
		}
	}
	h.windows[h.head][idx]++
}

func (h *serviceLatencyHeatmap) data() []LatencyHeatmapBucket {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Walk from oldest to newest
	buckets := make([]LatencyHeatmapBucket, 0, heatmapNumWindows)
	for i := 1; i <= heatmapNumWindows; i++ {
		idx := (h.head + i) % heatmapNumWindows
		if h.tsFor[idx] == 0 {
			continue
		}
		counts := make([]int64, heatmapNumBands)
		copy(counts, h.windows[idx][:])
		buckets = append(buckets, LatencyHeatmapBucket{
			Ts:     h.tsFor[idx] * heatmapWindowSec,
			Counts: counts,
		})
	}
	return buckets
}
