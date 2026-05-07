package sampler

import (
	"math"
	"sync"
	"time"
)

// RateLimitSampler uses a token bucket to limit sampling to N traces/sec.
type RateLimitSampler struct {
	mu           sync.Mutex
	tracesPerSec float64
	tokens       float64
	maxTokens    float64
	lastRefill   time.Time
}

// NewRateLimit creates a new RateLimitSampler.
func NewRateLimit(tracesPerSec float64) *RateLimitSampler {
	return &RateLimitSampler{
		tracesPerSec: tracesPerSec,
		tokens:       tracesPerSec * 2,
		maxTokens:    tracesPerSec * 2,
		lastRefill:   time.Now(),
	}
}

func (s *RateLimitSampler) ShouldSample(_ SamplingParameters) SamplingResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(s.lastRefill).Seconds()
	s.tokens = math.Min(s.maxTokens, s.tokens+elapsed*s.tracesPerSec)
	s.lastRefill = now
	if s.tokens >= 1.0 {
		s.tokens--
		return SamplingResult{Decision: Sample, Reason: "rate-limit granted"}
	}
	return SamplingResult{Decision: Drop, Reason: "rate-limit exceeded"}
}

func (s *RateLimitSampler) Name() string { return "ratelimit" }
func (s *RateLimitSampler) Config() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"tracesPerSec": s.tracesPerSec}
}
