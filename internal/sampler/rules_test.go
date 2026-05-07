package sampler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

func TestRuleBased_PriorityOrder(t *testing.T) {
	s := sampler.NewRuleBased([]sampler.Rule{
		{Priority: 0, Decision: sampler.Drop},
		{Priority: 100, Decision: sampler.Sample},
	}, sampler.NewNever())

	id, _ := model.NewTraceID()
	result := s.ShouldSample(sampler.SamplingParameters{TraceID: id})
	assert.Equal(t, sampler.Sample, result.Decision, "high-priority rule should win")
}

func TestRuleBased_Fallback(t *testing.T) {
	s := sampler.NewRuleBased([]sampler.Rule{
		{ServiceName: "other-svc", Priority: 100, Decision: sampler.Drop},
	}, sampler.NewAlways())

	id, _ := model.NewTraceID()
	result := s.ShouldSample(sampler.SamplingParameters{TraceID: id, ServiceName: "my-svc"})
	assert.Equal(t, sampler.Sample, result.Decision, "no match should use fallback")
}

func TestRuleBased_GlobMatching(t *testing.T) {
	s := sampler.NewRuleBased([]sampler.Rule{
		{OperationGlob: "HTTP GET *", Priority: 10, Decision: sampler.Sample},
	}, sampler.NewNever())

	id, _ := model.NewTraceID()
	r1 := s.ShouldSample(sampler.SamplingParameters{TraceID: id, OperationName: "HTTP GET /api/users"})
	assert.Equal(t, sampler.Sample, r1.Decision)

	r2 := s.ShouldSample(sampler.SamplingParameters{TraceID: id, OperationName: "HTTP POST /api/users"})
	assert.Equal(t, sampler.Drop, r2.Decision, "POST should not match GET glob")
}

func TestRuleBased_ServiceFilter(t *testing.T) {
	s := sampler.NewRuleBased([]sampler.Rule{
		{ServiceName: "payment-svc", Priority: 100, Decision: sampler.Sample},
	}, sampler.NewNever())

	id, _ := model.NewTraceID()
	r1 := s.ShouldSample(sampler.SamplingParameters{TraceID: id, ServiceName: "payment-svc"})
	assert.Equal(t, sampler.Sample, r1.Decision)

	r2 := s.ShouldSample(sampler.SamplingParameters{TraceID: id, ServiceName: "frontend-svc"})
	assert.Equal(t, sampler.Drop, r2.Decision)
}
