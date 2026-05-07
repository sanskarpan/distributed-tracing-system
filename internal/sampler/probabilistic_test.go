package sampler_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

func TestProbabilisticSampler_AllSampled(t *testing.T) {
	s := sampler.NewProbabilistic(1.0)
	for i := 0; i < 10000; i++ {
		id, err := model.NewTraceID()
		require.NoError(t, err)
		result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
		assert.Equal(t, sampler.Sample, result.Decision)
	}
}

func TestProbabilisticSampler_NonesSampled(t *testing.T) {
	s := sampler.NewProbabilistic(0.0)
	for i := 0; i < 10000; i++ {
		id, err := model.NewTraceID()
		require.NoError(t, err)
		result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
		assert.Equal(t, sampler.Drop, result.Decision)
	}
}

func TestProbabilisticSampler_RateAccuracy(t *testing.T) {
	s := sampler.NewProbabilistic(0.1)
	sampled := 0
	total := 100_000
	for i := 0; i < total; i++ {
		id, _ := model.NewTraceID()
		if s.ShouldSample(sampler.SamplingParameters{TraceID: id}).Decision == sampler.Sample {
			sampled++
		}
	}
	rate := float64(sampled) / float64(total)
	assert.InDelta(t, 0.10, rate, 0.005,
		"sampling rate should be ~10%%, got %.4f", rate)
}

func TestProbabilisticSampler_CrossServiceConsistency(t *testing.T) {
	rate := 0.3
	s1 := sampler.NewProbabilistic(rate)
	s2 := sampler.NewProbabilistic(rate)

	for i := 0; i < 10000; i++ {
		traceID, _ := model.NewTraceID()
		p := sampler.SamplingParameters{TraceID: traceID}

		result1 := s1.ShouldSample(p)
		result2 := s2.ShouldSample(p)

		assert.Equal(t, result1.Decision, result2.Decision,
			"services must agree on traceID %s: service1=%v service2=%v",
			traceID, result1.Decision, result2.Decision)
	}
}

func TestProbabilisticSampler_Determinism(t *testing.T) {
	s := sampler.NewProbabilistic(0.5)
	id, _ := model.NewTraceID()
	p := sampler.SamplingParameters{TraceID: id}
	first := s.ShouldSample(p).Decision
	for i := 0; i < 100; i++ {
		assert.Equal(t, first, s.ShouldSample(p).Decision)
	}
}

func TestProbabilisticSampler_ThreadSafety(t *testing.T) {
	s := sampler.NewProbabilistic(0.5)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _ := model.NewTraceID()
			s.ShouldSample(sampler.SamplingParameters{TraceID: id})
		}()
	}
	wg.Wait()
}

func TestProbabilisticSampler_SetRate(t *testing.T) {
	s := sampler.NewProbabilistic(1.0)
	s.SetRate(0.0)

	for i := 0; i < 1000; i++ {
		id, _ := model.NewTraceID()
		result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
		assert.Equal(t, sampler.Drop, result.Decision)
	}
}
