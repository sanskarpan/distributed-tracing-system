package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// HandleZipkinSpans accepts Zipkin v2 JSON format: POST /api/v2/spans
func (h *IngestHandler) HandleZipkinSpans(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var rawSpans []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&rawSpans); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}

	spans := make([]*model.Span, 0, len(rawSpans))
	tenantID := EffectiveTenant(PrincipalFromContext(r.Context()))
	for _, raw := range rawSpans {
		sp := parseZipkinSpan(raw)
		if sp != nil {
			sp.TenantID = tenantID
			spans = append(spans, sp)
		}
	}

	h.pipeline.IngestSpans(spans)
	if h.replicator != nil && r.Header.Get(replicationHeader) == "" {
		h.replicator.ReplicateAsync(spans, tenantID)
	}
	w.WriteHeader(http.StatusAccepted)
}

func parseZipkinSpan(raw map[string]any) *model.Span {
	traceIDStr, _ := raw["traceId"].(string)
	spanIDStr, _ := raw["id"].(string)

	traceID, err := model.ParseTraceID64Or128(traceIDStr)
	if err != nil {
		return nil
	}
	spanID, err := model.ParseSpanID(spanIDStr)
	if err != nil {
		return nil
	}

	sp := &model.Span{
		TraceID:     traceID,
		SpanID:      spanID,
		ServiceName: zipkinServiceName(raw),
		Name:        stringVal(raw, "name"),
	}

	if parentIDStr, ok := raw["parentId"].(string); ok && parentIDStr != "" {
		if pid, err := model.ParseSpanID(parentIDStr); err == nil {
			sp.ParentSpanID = pid
		}
	}

	// Timestamps: Zipkin uses microseconds
	if ts, ok := raw["timestamp"].(float64); ok {
		sp.StartTime = time.Unix(0, int64(ts)*int64(time.Microsecond))
	}
	if dur, ok := raw["duration"].(float64); ok && !sp.StartTime.IsZero() {
		sp.EndTime = sp.StartTime.Add(time.Duration(int64(dur)) * time.Microsecond)
	}

	// Kind
	if kind, ok := raw["kind"].(string); ok {
		switch kind {
		case "CLIENT":
			sp.Kind = model.SpanKindClient
		case "SERVER":
			sp.Kind = model.SpanKindServer
		case "PRODUCER":
			sp.Kind = model.SpanKindProducer
		case "CONSUMER":
			sp.Kind = model.SpanKindConsumer
		}
	}

	// Tags → attributes
	if tags, ok := raw["tags"].(map[string]any); ok {
		for k, v := range tags {
			if s, ok := v.(string); ok {
				sp.Attributes = append(sp.Attributes, model.StringKV(k, s))
			}
		}
	}

	// Error tag
	if tags, ok := raw["tags"].(map[string]any); ok {
		if errTag, hasErr := tags["error"]; hasErr && errTag != nil {
			sp.Status.Code = model.StatusError
			if msg, ok := errTag.(string); ok {
				sp.Status.Message = msg
			}
			sp.HasError = true
		}
	}

	if sp.StartTime.IsZero() {
		return nil
	}
	return sp
}

func zipkinServiceName(raw map[string]any) string {
	if ep, ok := raw["localEndpoint"].(map[string]any); ok {
		if sn, ok := ep["serviceName"].(string); ok && sn != "" {
			return sn
		}
	}
	return "unknown"
}
