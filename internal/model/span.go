package model

import "time"

// SpanKind describes the relationship between the span and its peers.
type SpanKind int

const (
	SpanKindInternal SpanKind = 1
	SpanKindServer   SpanKind = 2
	SpanKindClient   SpanKind = 3
	SpanKindProducer SpanKind = 4
	SpanKindConsumer SpanKind = 5
)

// StatusCode is the OTEL status code.
type StatusCode int

const (
	StatusUnset StatusCode = 0
	StatusOK    StatusCode = 1
	StatusError StatusCode = 2
)

// SpanStatus holds the status code and message for a span.
type SpanStatus struct {
	Code    StatusCode
	Message string
}

// SpanEvent is a timestamped annotation on a span.
type SpanEvent struct {
	Time       time.Time
	Name       string
	Attributes []KeyValue
}

// SpanLink is a reference to a span in another trace.
type SpanLink struct {
	TraceID    TraceID
	SpanID     SpanID
	TraceState string
	Attributes []KeyValue
}

// Span is the core data model for a single operation.
type Span struct {
	TraceID      TraceID
	SpanID       SpanID
	ParentSpanID SpanID // zero value = root span
	TraceState   string
	TenantID     string

	Name         string
	Kind         SpanKind
	ServiceName  string
	ServiceAttrs map[string]string

	StartTime time.Time
	EndTime   time.Time

	Attributes []KeyValue
	Events     []SpanEvent
	Links      []SpanLink
	Status     SpanStatus

	// Computed by assembler
	Depth    int
	HasError bool    // true if StatusError OR any descendant has error
	Children []*Span // populated during tree construction

	ReceivedAt time.Time
}

func (s *Span) Duration() time.Duration { return s.EndTime.Sub(s.StartTime) }
func (s *Span) IsRoot() bool            { return s.ParentSpanID.IsZero() }

// GetAttribute returns the value for the given key, or zero KeyValue if not found.
func (s *Span) GetAttribute(key string) (KeyValue, bool) {
	for _, kv := range s.Attributes {
		if kv.Key == key {
			return kv, true
		}
	}
	return KeyValue{}, false
}
