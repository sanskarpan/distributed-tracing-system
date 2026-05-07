package sampler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

// sampleN calls ShouldSample n times with random trace IDs, feeding the sliding window.
func sampleN(t *testing.T, s *sampler.AdaptiveSampler, n int) {
	t.Helper()
	p := sampler.SamplingParameters{}
	for i := 0; i < n; i++ {
		id, _ := model.NewTraceID()
		p.TraceID = id
		s.ShouldSample(p)
	}
}

func currentRate(s *sampler.AdaptiveSampler) float64 {
	v := s.Config()["currentRate"]
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// TestAdaptive_AtTarget verifies that when observed rate == target, the sampling rate is unchanged.
// target=100/s, we inject 6000 calls (6000/60 = 100/s), then wait 4 adjustment periods.
// Deviation < 10% → deviations counter stays 0 → no change.
func TestAdaptive_AtTarget(t *testing.T) {
	period := 80 * time.Millisecond
	a := sampler.NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()

	sampleN(t, a, 6000) // rate = 100/s

	before := currentRate(a)
	time.Sleep(5 * period) // 5 ticks, all within 10% → no adjustment
	after := currentRate(a)

	assert.InDelta(t, before, after, before*0.15,
		"rate should remain unchanged when observed==target (deviation<10%%)")
}

// TestAdaptive_DoubleRate verifies that 2x over-target triggers rate reduction after 3 ticks.
// target=100/s, observed=200/s → ratio=0.5 → rate halved after 3 consecutive deviations.
func TestAdaptive_DoubleRate(t *testing.T) {
	period := 80 * time.Millisecond
	a := sampler.NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()

	sampleN(t, a, 12000) // rate = 200/s

	before := currentRate(a)
	time.Sleep(5 * period) // 5 ticks → 3 deviations trigger adjustment
	after := currentRate(a)

	assert.Less(t, after, before*0.75,
		"rate should decrease when observed rate is 2x target")
}

// TestAdaptive_Hysteresis verifies that a single deviation does NOT trigger adjustment.
// After only 1 adjustment tick the deviations counter is 1 (< 3), so rate is unchanged.
func TestAdaptive_Hysteresis(t *testing.T) {
	period := 100 * time.Millisecond
	a := sampler.NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()

	sampleN(t, a, 12000) // rate = 200/s

	before := currentRate(a)
	time.Sleep(period + 20*time.Millisecond) // exactly 1 tick
	after := currentRate(a)

	assert.Equal(t, before, after,
		"single deviation should not trigger adjustment (needs 3 consecutive)")
}
