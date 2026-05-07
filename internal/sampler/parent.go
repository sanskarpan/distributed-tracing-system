package sampler

// ParentBasedSampler respects upstream sampling decisions for child spans,
// and delegates root spans to the root sampler.
type ParentBasedSampler struct {
	root                   Sampler
	remoteParentSampled    Sampler
	remoteParentNotSampled Sampler
}

// NewParentBased creates a ParentBasedSampler.
func NewParentBased(root, remoteParentSampled, remoteParentNotSampled Sampler) *ParentBasedSampler {
	return &ParentBasedSampler{
		root:                   root,
		remoteParentSampled:    remoteParentSampled,
		remoteParentNotSampled: remoteParentNotSampled,
	}
}

func (s *ParentBasedSampler) ShouldSample(p SamplingParameters) SamplingResult {
	if p.ParentSpanID.IsZero() {
		return s.root.ShouldSample(p)
	}
	if p.ParentSampled != nil {
		if *p.ParentSampled {
			return s.remoteParentSampled.ShouldSample(p)
		}
		return s.remoteParentNotSampled.ShouldSample(p)
	}
	return s.root.ShouldSample(p)
}

func (s *ParentBasedSampler) Name() string { return "parent_based" }
func (s *ParentBasedSampler) Config() map[string]any {
	return map[string]any{
		"root":                   s.root.Name(),
		"remoteParentSampled":    s.remoteParentSampled.Name(),
		"remoteParentNotSampled": s.remoteParentNotSampled.Name(),
	}
}
