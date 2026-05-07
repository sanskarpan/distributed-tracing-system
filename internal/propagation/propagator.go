package propagation

import (
	"net/http"

	"github.com/yourname/tracing/internal/model"
)

// SpanContext carries the tracing context across service boundaries.
type SpanContext struct {
	TraceID    model.TraceID
	SpanID     model.SpanID
	TraceState string
	IsSampled  bool
	IsRemote   bool
}

// IsValid returns true if both TraceID and SpanID are non-zero.
func (s SpanContext) IsValid() bool {
	return !s.TraceID.IsZero() && !s.SpanID.IsZero()
}

// Propagator injects and extracts span context from HTTP headers.
type Propagator interface {
	Inject(ctx SpanContext, headers http.Header)
	Extract(headers http.Header) (SpanContext, bool)
}
