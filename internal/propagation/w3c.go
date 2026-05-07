package propagation

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yourname/tracing/internal/model"
)

// W3CPropagator implements the W3C TraceContext propagation format.
type W3CPropagator struct{}

func (W3CPropagator) Inject(ctx SpanContext, headers http.Header) {
	flags := "00"
	if ctx.IsSampled {
		flags = "01"
	}
	headers.Set("traceparent", fmt.Sprintf("00-%s-%s-%s",
		ctx.TraceID.String(), ctx.SpanID.String(), flags))
	if ctx.TraceState != "" {
		headers.Set("tracestate", ctx.TraceState)
	}
}

func (W3CPropagator) Extract(headers http.Header) (SpanContext, bool) {
	val := headers.Get("traceparent")
	if val == "" {
		return SpanContext{}, false
	}
	parts := strings.SplitN(val, "-", 4)
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return SpanContext{}, false
	}
	traceID, err1 := model.ParseTraceID(parts[1])
	spanID, err2 := model.ParseSpanID(parts[2])
	if err1 != nil || err2 != nil {
		return SpanContext{}, false
	}
	// flags: last character's LSB is the sampled bit
	flagStr := parts[3]
	sampled := len(flagStr) >= 2 && flagStr[len(flagStr)-1]&1 != 0
	return SpanContext{
		TraceID:    traceID,
		SpanID:     spanID,
		IsSampled:  sampled,
		TraceState: headers.Get("tracestate"),
		IsRemote:   true,
	}, true
}
