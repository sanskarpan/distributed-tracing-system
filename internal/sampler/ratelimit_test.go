package sampler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

func TestRateLimit_BurstThenDrop(t *testing.T) {
	// 10/sec → maxTokens = 20 (burst)
	s := sampler.NewRateLimit(10)
	p := sampler.SamplingParameters{}

	// First 20 should be sampled (burst)
	sampled := 0
	for i := 0; i < 50; i++ {
		if s.ShouldSample(p).Decision == sampler.Sample {
			sampled++
		}
	}
	assert.Equal(t, 20, sampled, "burst capacity should allow 20 (2× rate)")
}

func TestRateLimit_RefillsOverTime(t *testing.T) {
	s := sampler.NewRateLimit(100)
	p := sampler.SamplingParameters{}

	// Drain the bucket
	for i := 0; i < 300; i++ {
		s.ShouldSample(p)
	}

	// Sleep 100ms → expect ~10 tokens refilled (100/sec × 0.1s)
	time.Sleep(100 * time.Millisecond)

	sampled := 0
	for i := 0; i < 20; i++ {
		if s.ShouldSample(p).Decision == sampler.Sample {
			sampled++
		}
	}
	assert.Greater(t, sampled, 0, "tokens should have refilled after sleep")
}

func TestParentBased_RootAlways(t *testing.T) {
	s := sampler.NewParentBased(sampler.NewAlways(), sampler.NewAlways(), sampler.NewNever())
	id, _ := model.NewTraceID()
	// Root span (no parent)
	result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
	assert.Equal(t, sampler.Sample, result.Decision)
}

func TestParentBased_RootNever(t *testing.T) {
	s := sampler.NewParentBased(sampler.NewNever(), sampler.NewAlways(), sampler.NewNever())
	id, _ := model.NewTraceID()
	result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
	assert.Equal(t, sampler.Drop, result.Decision)
}

func TestParentBased_ParentSampledTrue(t *testing.T) {
	s := sampler.NewParentBased(sampler.NewNever(), sampler.NewAlways(), sampler.NewNever())
	traceID, _ := model.NewTraceID()
	parentID, _ := model.NewSpanID()
	sampled := true
	result := s.ShouldSample(sampler.SamplingParameters{
		TraceID:       traceID,
		ParentSpanID:  parentID,
		ParentSampled: &sampled,
	})
	assert.Equal(t, sampler.Sample, result.Decision)
}

func TestParentBased_ParentSampledFalse(t *testing.T) {
	s := sampler.NewParentBased(sampler.NewAlways(), sampler.NewAlways(), sampler.NewNever())
	traceID, _ := model.NewTraceID()
	parentID, _ := model.NewSpanID()
	sampled := false
	result := s.ShouldSample(sampler.SamplingParameters{
		TraceID:       traceID,
		ParentSpanID:  parentID,
		ParentSampled: &sampled,
	})
	assert.Equal(t, sampler.Drop, result.Decision)
}
