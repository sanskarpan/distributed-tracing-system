package propagation

import (
	"encoding/hex"
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
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return SpanContext{}, false
	}
	traceID, err1 := model.ParseTraceID(parts[1])
	spanID, err2 := model.ParseSpanID(parts[2])
	flags, err3 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil || err3 != nil || len(flags) != 1 {
		return SpanContext{}, false
	}
	sampled := flags[0]&0x01 == 0x01
	return SpanContext{
		TraceID:    traceID,
		SpanID:     spanID,
		IsSampled:  sampled,
		TraceState: headers.Get("tracestate"),
		IsRemote:   true,
	}, true
}
