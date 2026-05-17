package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yourname/tracing/internal/model"
)

type IngestHandler struct {
	pipeline *Pipeline
}

func NewIngestHandler(pipeline *Pipeline) *IngestHandler {
	return &IngestHandler{pipeline: pipeline}
}

// HandleNativeSpans handles POST /api/v1/spans
func (h *IngestHandler) HandleNativeSpans(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}

	spans := make([]*model.Span, 0, len(req.Spans))
	parseDropped := 0
	tenantID := EffectiveTenant(PrincipalFromContext(r.Context()))
	for _, dto := range req.Spans {
		sp, err := dtoToSpan(dto)
		if err != nil {
			parseDropped++
			continue
		}
		sp.TenantID = tenantID
		spans = append(spans, sp)
	}

	accepted, dropped, _ := h.pipeline.IngestSpans(spans)
	dropped += parseDropped

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(IngestResponse{Accepted: accepted, Dropped: dropped})
}

// HandleOTLPTraces handles POST /v1/traces (OTLP HTTP/JSON)
func (h *IngestHandler) HandleOTLPTraces(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}

	spans, err := parseOTLPBody(body)
	if err != nil {
		http.Error(w, `{"error":"invalid OTLP"}`, 400)
		return
	}

	tenantID := EffectiveTenant(PrincipalFromContext(r.Context()))
	for _, sp := range spans {
		sp.TenantID = tenantID
	}

	h.pipeline.IngestSpans(spans)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

func dtoToSpan(dto SpanDTO) (*model.Span, error) {
	traceID, err := model.ParseTraceID(dto.TraceID)
	if err != nil {
		return nil, err
	}
	spanID, err := model.ParseSpanID(dto.SpanID)
	if err != nil {
		return nil, err
	}

	if dto.StartTimeUnixNano == 0 {
		return nil, fmt.Errorf("startTimeUnixNano must be non-zero")
	}
	if dto.EndTimeUnixNano < dto.StartTimeUnixNano {
		return nil, fmt.Errorf("endTimeUnixNano must be >= startTimeUnixNano")
	}

	// Enforce field length limits
	if len(dto.ServiceName) > 256 {
		return nil, fmt.Errorf("serviceName exceeds 256 characters")
	}
	if len(dto.Name) > 256 {
		return nil, fmt.Errorf("span name exceeds 256 characters")
	}
	if len(dto.Attributes) > 64 {
		return nil, fmt.Errorf("too many span attributes (max 64)")
	}
	for _, kv := range dto.Attributes {
		if len(kv.Key) > 128 {
			return nil, fmt.Errorf("attribute key %q exceeds 128 characters", kv.Key)
		}
		if kv.StringValue != nil && len(*kv.StringValue) > 1024 {
			return nil, fmt.Errorf("attribute %q value exceeds 1024 characters", kv.Key)
		}
	}

	sp := &model.Span{
		TraceID:      traceID,
		SpanID:       spanID,
		Name:         dto.Name,
		Kind:         model.SpanKind(dto.Kind),
		ServiceName:  dto.ServiceName,
		ServiceAttrs: dto.ServiceAttributes,
		StartTime:    nanoToTime(dto.StartTimeUnixNano),
		EndTime:      nanoToTime(dto.EndTimeUnixNano),
		Attributes:   dtoToAttributes(dto.Attributes),
		Status: model.SpanStatus{
			Code:    model.StatusCode(dto.Status.Code),
			Message: dto.Status.Message,
		},
	}

	if dto.ParentSpanID != "" {
		parentID, err := model.ParseSpanID(dto.ParentSpanID)
		if err == nil {
			sp.ParentSpanID = parentID
		}
	}

	for _, e := range dto.Events {
		sp.Events = append(sp.Events, model.SpanEvent{
			Time:       nanoToTime(e.TimeUnixNano),
			Name:       e.Name,
			Attributes: dtoToAttributes(e.Attributes),
		})
	}

	for _, l := range dto.Links {
		traceID, err1 := model.ParseTraceID(l.TraceID)
		spanID, err2 := model.ParseSpanID(l.SpanID)
		if err1 == nil && err2 == nil {
			sp.Links = append(sp.Links, model.SpanLink{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceState: l.TraceState,
				Attributes: dtoToAttributes(l.Attributes),
			})
		}
	}

	return sp, nil
}

func nanoToTime(ns uint64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(ns))
}

func parseOTLPBody(body map[string]any) ([]*model.Span, error) {
	resourceSpans, _ := body["resourceSpans"].([]any)
	var spans []*model.Span

	for _, rsAny := range resourceSpans {
		rs, ok := rsAny.(map[string]any)
		if !ok {
			continue
		}

		serviceName := "unknown"
		if resource, ok := rs["resource"].(map[string]any); ok {
			if attrs, ok := resource["attributes"].([]any); ok {
				serviceName = extractOTLPServiceName(attrs)
			}
		}

		scopeSpans, _ := rs["scopeSpans"].([]any)
		for _, ssAny := range scopeSpans {
			ss, ok := ssAny.(map[string]any)
			if !ok {
				continue
			}
			spansAny, _ := ss["spans"].([]any)
			for _, spAny := range spansAny {
				sp, ok := spAny.(map[string]any)
				if !ok {
					continue
				}
				span := parseOTLPSpan(sp, serviceName)
				if span != nil {
					spans = append(spans, span)
				}
			}
		}
	}
	return spans, nil
}

func extractOTLPServiceName(attrs []any) string {
	for _, attrAny := range attrs {
		attr, ok := attrAny.(map[string]any)
		if !ok {
			continue
		}
		if key, _ := attr["key"].(string); key == "service.name" {
			if val, ok := attr["value"].(map[string]any); ok {
				if sv, ok := val["stringValue"].(string); ok {
					return sv
				}
			}
		}
	}
	return "unknown"
}

func parseOTLPSpan(sp map[string]any, serviceName string) *model.Span {
	traceIDStr, _ := sp["traceId"].(string)
	spanIDStr, _ := sp["spanId"].(string)

	traceID, err := model.ParseTraceID(traceIDStr)
	if err != nil {
		return nil
	}
	spanID, err := model.ParseSpanID(spanIDStr)
	if err != nil {
		return nil
	}

	span := &model.Span{
		TraceID:     traceID,
		SpanID:      spanID,
		Name:        stringVal(sp, "name"),
		ServiceName: serviceName,
	}

	if parentIDStr, ok := sp["parentSpanId"].(string); ok && parentIDStr != "" {
		if parentID, err := model.ParseSpanID(parentIDStr); err == nil {
			span.ParentSpanID = parentID
		}
	}

	if kind, ok := sp["kind"].(float64); ok {
		span.Kind = model.SpanKind(int(kind))
	}

	// Parse timestamps (nanoseconds as string or number)
	if v, ok := sp["startTimeUnixNano"]; ok {
		span.StartTime = nanoToTime(parseUint64(v))
	}
	if v, ok := sp["endTimeUnixNano"]; ok {
		span.EndTime = nanoToTime(parseUint64(v))
	}

	// Parse attributes
	if attrs, ok := sp["attributes"].([]any); ok {
		for _, attrAny := range attrs {
			if attr, ok := attrAny.(map[string]any); ok {
				if kv := parseOTLPAttribute(attr); kv.Key != "" {
					span.Attributes = append(span.Attributes, kv)
				}
			}
		}
	}

	// Parse status
	if status, ok := sp["status"].(map[string]any); ok {
		if code, ok := status["code"].(float64); ok {
			span.Status.Code = model.StatusCode(int(code))
		}
		if msg, ok := status["message"].(string); ok {
			span.Status.Message = msg
		}
	}

	return span
}

func parseOTLPAttribute(attr map[string]any) model.KeyValue {
	key, _ := attr["key"].(string)
	if key == "" {
		return model.KeyValue{}
	}
	val, ok := attr["value"].(map[string]any)
	if !ok {
		return model.KeyValue{}
	}
	if sv, ok := val["stringValue"].(string); ok {
		return model.StringKV(key, sv)
	}
	if iv, ok := val["intValue"]; ok {
		switch x := iv.(type) {
		case float64:
			return model.IntKV(key, int64(x))
		case string:
			n, _ := strconv.ParseInt(x, 10, 64)
			return model.IntKV(key, n)
		}
	}
	if bv, ok := val["boolValue"].(bool); ok {
		return model.BoolKV(key, bv)
	}
	if fv, ok := val["doubleValue"].(float64); ok {
		return model.FloatKV(key, fv)
	}
	return model.KeyValue{}
}

func parseUint64(v any) uint64 {
	switch x := v.(type) {
	case float64:
		return uint64(x)
	case string:
		n, _ := strconv.ParseUint(x, 10, 64)
		return n
	}
	return 0
}

func stringVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
