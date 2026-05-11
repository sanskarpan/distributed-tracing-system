package sampler

import (
	"math"
	"sync"
	"time"
)

// AdaptiveSampler automatically adjusts sampling probability to maintain target throughput.
type AdaptiveSampler struct {
	mu            sync.Mutex
	targetRate    float64
	minRate       float64
	maxRate       float64
	adjustPeriod  time.Duration
	inner         *ProbabilisticSampler
	attemptWindow *SlidingWindow
	sampledWindow *SlidingWindow
	deviations    int
	stopCh        chan struct{}
}

// NewAdaptive creates an AdaptiveSampler targeting the given traces/second.
func NewAdaptive(targetRate, minRate, maxRate float64, adjustPeriod time.Duration) *AdaptiveSampler {
	if minRate <= 0 {
		minRate = 0.001
	}
	if maxRate <= 0 || maxRate > 1 {
		maxRate = 1.0
	}
	if adjustPeriod <= 0 {
		adjustPeriod = 5 * time.Second
	}
	initialRate := math.Min(maxRate, math.Max(minRate, 0.1))
	a := &AdaptiveSampler{
		targetRate:    targetRate,
		minRate:       minRate,
		maxRate:       maxRate,
		adjustPeriod:  adjustPeriod,
		inner:         NewProbabilistic(initialRate),
		attemptWindow: &SlidingWindow{},
		sampledWindow: &SlidingWindow{},
		stopCh:        make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *AdaptiveSampler) run() {
	ticker := time.NewTicker(a.adjustPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.adjust()
		case <-a.stopCh:
			return
		}
	}
}

func (a *AdaptiveSampler) adjust() {
	attempted := a.attemptWindow.Rate()
	if attempted == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	sampled := a.sampledWindow.Rate()
	if sampled == 0 {
		a.deviations++
		if a.deviations < 3 {
			return
		}
		a.inner.SetRate(a.maxRate)
		a.deviations = 0
		return
	}

	ratio := a.targetRate / sampled
	deviation := math.Abs(ratio - 1.0)
	if deviation < 0.10 {
		a.deviations = 0
		return
	}
	a.deviations++
	if a.deviations < 3 {
		return
	}
	newRate := a.inner.GetRate() * ratio
	newRate = math.Max(a.minRate, math.Min(a.maxRate, newRate))
	a.inner.SetRate(newRate)
	a.deviations = 0
}

func (a *AdaptiveSampler) ShouldSample(p SamplingParameters) SamplingResult {
	a.attemptWindow.Add(1)
	result := a.inner.ShouldSample(p)
	if result.Decision != Drop {
		a.sampledWindow.Add(1)
	}
	return result
}

// Stop shuts down the background adjustment goroutine.
func (a *AdaptiveSampler) Stop() {
	close(a.stopCh)
}

func (a *AdaptiveSampler) Name() string { return "adaptive" }
func (a *AdaptiveSampler) Config() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{
		"targetRate":  a.targetRate,
		"currentRate": a.inner.GetRate(),
		"minRate":     a.minRate,
		"maxRate":     a.maxRate,
		"attemptRate": a.attemptWindow.Rate(),
		"sampledRate": a.sampledWindow.Rate(),
	}
}
