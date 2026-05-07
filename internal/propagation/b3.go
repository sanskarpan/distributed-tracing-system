package propagation

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yourname/tracing/internal/model"
)

// B3Propagator implements B3 multi-header or single-header propagation.
type B3Propagator struct {
	SingleHeader bool
}

func (p B3Propagator) Inject(ctx SpanContext, headers http.Header) {
	if p.SingleHeader {
		sampled := "0"
		if ctx.IsSampled {
			sampled = "1"
		}
		headers.Set("b3", fmt.Sprintf("%s-%s-%s",
			ctx.TraceID.String(), ctx.SpanID.String(), sampled))
		return
	}
	// Multi-header
	headers.Set("X-B3-TraceId", ctx.TraceID.String())
	headers.Set("X-B3-SpanId", ctx.SpanID.String())
	if ctx.IsSampled {
		headers.Set("X-B3-Sampled", "1")
	} else {
		headers.Set("X-B3-Sampled", "0")
	}
}

func (p B3Propagator) Extract(headers http.Header) (SpanContext, bool) {
	if p.SingleHeader {
		return p.extractSingle(headers)
	}
	return p.extractMulti(headers)
}

func (p B3Propagator) extractMulti(headers http.Header) (SpanContext, bool) {
	traceIDStr := headers.Get("X-B3-TraceId")
	spanIDStr := headers.Get("X-B3-SpanId")
	if traceIDStr == "" || spanIDStr == "" {
		return SpanContext{}, false
	}
	traceID, err := model.ParseTraceID(traceIDStr)
	if err != nil {
		return SpanContext{}, false
	}
	spanID, err := model.ParseSpanID(spanIDStr)
	if err != nil {
		return SpanContext{}, false
	}
	sampled := headers.Get("X-B3-Sampled") == "1"
	return SpanContext{
		TraceID:   traceID,
		SpanID:    spanID,
		IsSampled: sampled,
		IsRemote:  true,
	}, true
}

func (p B3Propagator) extractSingle(headers http.Header) (SpanContext, bool) {
	val := headers.Get("b3")
	if val == "" {
		return SpanContext{}, false
	}
	parts := strings.Split(val, "-")
	if len(parts) < 3 {
		return SpanContext{}, false
	}
	traceID, err := model.ParseTraceID(parts[0])
	if err != nil {
		return SpanContext{}, false
	}
	spanID, err := model.ParseSpanID(parts[1])
	if err != nil {
		return SpanContext{}, false
	}
	sampled := parts[2] == "1"
	return SpanContext{
		TraceID:   traceID,
		SpanID:    spanID,
		IsSampled: sampled,
		IsRemote:  true,
	}, true
}
