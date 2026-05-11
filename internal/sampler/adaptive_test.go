package sampler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourname/tracing/internal/model"
)

// sampleN calls ShouldSample n times with random trace IDs, feeding the sliding window.
func sampleN(t *testing.T, s *AdaptiveSampler, n int) {
	t.Helper()
	p := SamplingParameters{}
	for i := 0; i < n; i++ {
		id, _ := model.NewTraceID()
		p.TraceID = id
		s.ShouldSample(p)
	}
}

func currentRate(s *AdaptiveSampler) float64 {
	v := s.Config()["currentRate"]
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// TestAdaptive_TracksSampledOutput verifies that the controller regulates sampled
// output, not ingress. With 1000 attempted traces/sec and an initial 10% rate,
// sampled throughput is already ~100/sec, so the rate should stay stable.
func TestAdaptive_TracksSampledOutput(t *testing.T) {
	period := 80 * time.Millisecond
	a := NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()

	sampleN(t, a, 60000) // attempted rate = 1000/s, sampled rate ~= 100/s

	before := currentRate(a)
	time.Sleep(5 * period) // 5 ticks, all within 10% → no adjustment
	after := currentRate(a)

	assert.InDelta(t, before, after, before*0.15,
		"rate should remain unchanged when sampled output already matches target")
}

// TestAdaptive_OverTargetSampledRate verifies that the controller reduces the rate
// when sampled output is above target.
func TestAdaptive_OverTargetSampledRate(t *testing.T) {
	period := 80 * time.Millisecond
	a := NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()
	a.inner.SetRate(0.5)

	sampleN(t, a, 24000) // attempted rate = 400/s, sampled rate ~= 200/s at 50%

	before := currentRate(a)
	time.Sleep(5 * period) // 5 ticks → 3 deviations trigger adjustment
	after := currentRate(a)

	assert.Less(t, after, before*0.75,
		"rate should decrease when sampled output is 2x target")
}

// TestAdaptive_Hysteresis verifies that a single deviation does NOT trigger adjustment.
// After only 1 adjustment tick the deviations counter is 1 (< 3), so rate is unchanged.
func TestAdaptive_Hysteresis(t *testing.T) {
	period := 100 * time.Millisecond
	a := NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()
	a.inner.SetRate(0.5)

	sampleN(t, a, 24000) // sampled rate ~= 200/s at 50%

	before := currentRate(a)
	time.Sleep(period + 20*time.Millisecond) // exactly 1 tick
	after := currentRate(a)

	assert.Equal(t, before, after,
		"single deviation should not trigger adjustment (needs 3 consecutive)")
}

func TestAdaptive_ConfigExposesAttemptAndSampledRates(t *testing.T) {
	period := 80 * time.Millisecond
	a := NewAdaptive(100.0, 0.001, 1.0, period)
	defer a.Stop()

	sampleN(t, a, 6000)

	cfg := a.Config()
	attemptRate, ok := cfg["attemptRate"].(float64)
	assert.True(t, ok)
	sampledRate, ok := cfg["sampledRate"].(float64)
	assert.True(t, ok)
	assert.Greater(t, attemptRate, sampledRate)
	assert.Greater(t, sampledRate, 0.0)
}
