package metrics

import (
	"math/rand"
	"sort"
	"sync"
)

// Histogram is a reservoir-sampling histogram for tracking duration distributions.
type Histogram struct {
	mu      sync.Mutex
	samples []float64
	n       int64
	cap     int
}

// NewHistogram creates a new Histogram with the given reservoir capacity.
func NewHistogram(cap int) *Histogram { return &Histogram{cap: cap} }

// Record adds a new sample to the histogram using reservoir sampling.
func (h *Histogram) Record(ms float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.n++
	if int(h.n) <= h.cap {
		h.samples = append(h.samples, ms)
	} else {
		j := rand.Int63n(h.n)
		if j < int64(h.cap) {
			h.samples[j] = ms
		}
	}
}

// Percentile returns the value at the given percentile p (0.0–1.0).
func (h *Histogram) Percentile(p float64) float64 {
	h.mu.Lock()
	sorted := append([]float64{}, h.samples...)
	h.mu.Unlock()
	sort.Float64s(sorted)
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func (h *Histogram) P50() float64  { return h.Percentile(0.50) }
func (h *Histogram) P95() float64  { return h.Percentile(0.95) }
func (h *Histogram) P99() float64  { return h.Percentile(0.99) }
func (h *Histogram) P999() float64 { return h.Percentile(0.999) }
