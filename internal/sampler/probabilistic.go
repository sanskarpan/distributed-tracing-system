package sampler

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

// ProbabilisticSampler samples based on a deterministic hash of the TraceID.
// CRITICAL: Uses TraceID as input so all services agree on the same decision.
type ProbabilisticSampler struct {
	mu        sync.RWMutex
	rate      float64
	threshold uint64 // uint64(rate * math.MaxUint64)
}

// NewProbabilistic creates a new ProbabilisticSampler with the given rate [0.0, 1.0].
func NewProbabilistic(rate float64) *ProbabilisticSampler {
	rate = math.Max(0, math.Min(1, rate))
	return &ProbabilisticSampler{
		rate:      rate,
		threshold: uint64(rate * float64(math.MaxUint64)),
	}
}

func (s *ProbabilisticSampler) ShouldSample(p SamplingParameters) SamplingResult {
	id := binary.BigEndian.Uint64(p.TraceID[:8])
	s.mu.RLock()
	threshold := s.threshold
	rate := s.rate
	s.mu.RUnlock()
	if id < threshold {
		return SamplingResult{Decision: Sample, Reason: fmt.Sprintf("probabilistic rate=%.4f", rate)}
	}
	return SamplingResult{Decision: Drop, Reason: "probabilistic dropped"}
}

// SetRate updates the sampling rate thread-safely.
func (s *ProbabilisticSampler) SetRate(rate float64) {
	s.mu.Lock()
	s.rate = math.Max(0, math.Min(1, rate))
	s.threshold = uint64(s.rate * float64(math.MaxUint64))
	s.mu.Unlock()
}

// GetRate returns the current sampling rate.
func (s *ProbabilisticSampler) GetRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rate
}

func (s *ProbabilisticSampler) Name() string { return "probabilistic" }
func (s *ProbabilisticSampler) Config() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{"rate": s.rate}
}
