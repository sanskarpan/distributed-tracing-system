package sampler

import (
	"sync"
	"time"
)

const windowBuckets = 60

// SlidingWindow tracks a rate over a 60-second sliding window.
type SlidingWindow struct {
	mu      sync.Mutex
	buckets [windowBuckets]int64
	lastIdx int
	lastSec int64
}

// Add increments the current bucket by n.
func (w *SlidingWindow) Add(n int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().Unix()
	w.advance(now)
	w.buckets[w.lastIdx] += n
}

// Rate returns the per-second rate over the last 60 seconds.
func (w *SlidingWindow) Rate() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now().Unix()
	w.advance(now)
	var total int64
	for _, v := range w.buckets {
		total += v
	}
	return float64(total) / float64(windowBuckets)
}

func (w *SlidingWindow) advance(nowSec int64) {
	if w.lastSec == 0 {
		w.lastSec = nowSec
		return
	}
	diff := nowSec - w.lastSec
	if diff <= 0 {
		return
	}
	if diff >= windowBuckets {
		// clear all
		for i := range w.buckets {
			w.buckets[i] = 0
		}
		w.lastIdx = int(nowSec % windowBuckets)
		w.lastSec = nowSec
		return
	}
	for i := int64(1); i <= diff; i++ {
		idx := int((w.lastSec + i) % windowBuckets)
		w.buckets[idx] = 0
	}
	w.lastIdx = int(nowSec % windowBuckets)
	w.lastSec = nowSec
}
