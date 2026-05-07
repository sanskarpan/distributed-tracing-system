package propagation

import "net/http"

// CompositePropagator tries W3C first, then B3 multi, then B3 single.
type CompositePropagator struct {
	propagators []Propagator
}

// NewCompositePropagator returns a propagator that tries W3C, then B3 multi, then B3 single.
func NewCompositePropagator() *CompositePropagator {
	return &CompositePropagator{
		propagators: []Propagator{
			W3CPropagator{},
			B3Propagator{SingleHeader: false},
			B3Propagator{SingleHeader: true},
		},
	}
}

func (c *CompositePropagator) Inject(ctx SpanContext, headers http.Header) {
	// Inject with the first (highest priority) propagator
	c.propagators[0].Inject(ctx, headers)
}

func (c *CompositePropagator) Extract(headers http.Header) (SpanContext, bool) {
	for _, p := range c.propagators {
		if ctx, ok := p.Extract(headers); ok {
			return ctx, true
		}
	}
	return SpanContext{}, false
}
