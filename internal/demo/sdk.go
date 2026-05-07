package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/propagation"
)

type contextKey struct{}

// DemoSDK is a minimal tracing SDK for demo services.
type DemoSDK struct {
	serviceName  string
	collectorURL string
	propagator   propagation.Propagator
	httpClient   *http.Client

	mu    sync.Mutex
	spans []*model.Span
}

func NewDemoSDK(serviceName, collectorURL string) *DemoSDK {
	return &DemoSDK{
		serviceName:  serviceName,
		collectorURL: collectorURL,
		propagator:   propagation.W3CPropagator{},
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

// SpanOption is a functional option for span creation.
type SpanOption func(*model.Span)

// WithService overrides the service name for this span.
func WithService(name string) SpanOption {
	return func(s *model.Span) { s.ServiceName = name }
}

// StartSpan creates a new span and attaches it to the context.
func (s *DemoSDK) StartSpan(ctx context.Context, name string, kind model.SpanKind, opts ...SpanOption) (context.Context, *model.Span) {
	traceID, _ := model.NewTraceID()
	spanID, _ := model.NewSpanID()

	span := &model.Span{
		TraceID:     traceID,
		SpanID:      spanID,
		Name:        name,
		Kind:        kind,
		ServiceName: s.serviceName,
		StartTime:   time.Now(),
	}

	// Inherit trace context from parent span in ctx
	if parent, ok := ctx.Value(contextKey{}).(*model.Span); ok && parent != nil {
		span.TraceID = parent.TraceID
		span.ParentSpanID = parent.SpanID
	}

	for _, opt := range opts {
		opt(span)
	}

	return context.WithValue(ctx, contextKey{}, span), span
}

// FinishSpan sets the end time and records the span.
func (s *DemoSDK) FinishSpan(span *model.Span) {
	if span.EndTime.IsZero() {
		span.EndTime = time.Now()
	}
	s.mu.Lock()
	s.spans = append(s.spans, span)
	s.mu.Unlock()
}

// AddEvent adds a timestamped event to a span.
func (s *DemoSDK) AddEvent(span *model.Span, name string, attrs ...model.KeyValue) {
	span.Events = append(span.Events, model.SpanEvent{
		Time:       time.Now(),
		Name:       name,
		Attributes: attrs,
	})
}

// SetError sets the span status to error.
func (s *DemoSDK) SetError(span *model.Span, err error) {
	span.Status = model.SpanStatus{Code: model.StatusError, Message: err.Error()}
	span.HasError = true
}

// InjectHTTP injects the current span context into HTTP headers.
func (s *DemoSDK) InjectHTTP(ctx context.Context, headers http.Header) {
	if span, ok := ctx.Value(contextKey{}).(*model.Span); ok && span != nil {
		sc := propagation.SpanContext{
			TraceID:   span.TraceID,
			SpanID:    span.SpanID,
			IsSampled: true,
		}
		s.propagator.Inject(sc, headers)
	}
}

// ExtractHTTP extracts span context from HTTP headers and returns a context with parent span info.
func (s *DemoSDK) ExtractHTTP(ctx context.Context, headers http.Header) context.Context {
	sc, ok := s.propagator.Extract(headers)
	if !ok {
		return ctx
	}
	parent := &model.Span{
		TraceID: sc.TraceID,
		SpanID:  sc.SpanID,
	}
	return context.WithValue(ctx, contextKey{}, parent)
}

// Export POSTs all buffered spans to the collector and clears the buffer.
func (s *DemoSDK) Export() error {
	s.mu.Lock()
	spans := s.spans
	s.spans = nil
	s.mu.Unlock()

	if len(spans) == 0 {
		return nil
	}

	dtos := make([]map[string]any, 0, len(spans))
	for _, sp := range spans {
		attrs := make([]map[string]any, 0, len(sp.Attributes))
		for _, kv := range sp.Attributes {
			attr := map[string]any{"key": kv.Key}
			switch kv.Type {
			case model.ValueString:
				attr["stringValue"] = kv.SVal
			case model.ValueInt:
				attr["intValue"] = kv.IVal
			case model.ValueBool:
				attr["boolValue"] = kv.BVal
			case model.ValueFloat:
				attr["doubleValue"] = kv.FVal
			}
			attrs = append(attrs, attr)
		}
		dto := map[string]any{
			"traceId":           sp.TraceID.String(),
			"spanId":            sp.SpanID.String(),
			"parentSpanId":      sp.ParentSpanID.String(),
			"name":              sp.Name,
			"kind":              int(sp.Kind),
			"serviceName":       sp.ServiceName,
			"serviceAttributes": sp.ServiceAttrs,
			"startTimeUnixNano": uint64(sp.StartTime.UnixNano()),
			"endTimeUnixNano":   uint64(sp.EndTime.UnixNano()),
			"attributes":        attrs,
			"events":            []any{},
			"links":             []any{},
			"status":            map[string]any{"code": int(sp.Status.Code), "message": sp.Status.Message},
		}
		dtos = append(dtos, dto)
	}

	body, _ := json.Marshal(map[string]any{"spans": dtos})
	resp, err := s.httpClient.Post(s.collectorURL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}
	resp.Body.Close()
	return nil
}

// SpanFromContext returns the active span from context.
func SpanFromContext(ctx context.Context) *model.Span {
	sp, _ := ctx.Value(contextKey{}).(*model.Span)
	return sp
}
