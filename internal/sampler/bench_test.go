package sampler_test

import (
	"testing"
	"time"

	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

func BenchmarkTailSampler_AddSpan(b *testing.B) {
	accepted := 0
	ts := sampler.NewTailSampler(
		10*time.Millisecond,
		10000,
		[]sampler.TailPolicy{sampler.ErrorPolicy{}},
		func(spans []*model.Span) { accepted++ },
		func(model.TraceID) {},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		traceID, _ := model.NewTraceID()
		spanID, _ := model.NewSpanID()
		ts.AddSpan(&model.Span{
			TraceID:   traceID,
			SpanID:    spanID,
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Millisecond),
		})
	}
	ts.Stop()
}

func BenchmarkProbabilisticSampler_ShouldSample(b *testing.B) {
	s := sampler.NewProbabilistic(0.5)
	id, _ := model.NewTraceID()
	p := sampler.SamplingParameters{TraceID: id}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.ShouldSample(p)
	}
}
