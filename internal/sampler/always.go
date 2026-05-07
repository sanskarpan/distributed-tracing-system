package sampler

// AlwaysSampler samples every span.
type AlwaysSampler struct{}

func NewAlways() *AlwaysSampler { return &AlwaysSampler{} }

func (AlwaysSampler) ShouldSample(_ SamplingParameters) SamplingResult {
	return SamplingResult{Decision: Sample, Reason: "always"}
}

func (AlwaysSampler) Name() string          { return "always" }
func (AlwaysSampler) Config() map[string]any { return map[string]any{} }

// NeverSampler drops every span.
type NeverSampler struct{}

func NewNever() *NeverSampler { return &NeverSampler{} }

func (NeverSampler) ShouldSample(_ SamplingParameters) SamplingResult {
	return SamplingResult{Decision: Drop, Reason: "never"}
}

func (NeverSampler) Name() string          { return "never" }
func (NeverSampler) Config() map[string]any { return map[string]any{} }
