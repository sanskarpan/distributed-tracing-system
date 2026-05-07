package processor

import (
	"time"

	"github.com/yourname/tracing/internal/model"
)

// Enricher adds computed fields to spans before they are stored.
type Enricher struct{}

// Enrich sets HasError, ReceivedAt, and extracts ServiceName from service attributes.
func (e *Enricher) Enrich(span *model.Span) {
	// Set HasError based on status code
	span.HasError = span.Status.Code == model.StatusError

	// Set ReceivedAt to current time
	span.ReceivedAt = time.Now()

	// Extract ServiceName from ServiceAttrs["service.name"] if not already set
	if span.ServiceName == "" {
		if name, ok := span.ServiceAttrs["service.name"]; ok {
			span.ServiceName = name
		}
	}
}
