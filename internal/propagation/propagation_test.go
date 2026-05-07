package propagation_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/propagation"
)

func newTestContext(t *testing.T) propagation.SpanContext {
	t.Helper()
	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)
	return propagation.SpanContext{
		TraceID:   traceID,
		SpanID:    spanID,
		IsSampled: true,
	}
}

func TestW3C_InjectExtractRoundtrip(t *testing.T) {
	p := propagation.W3CPropagator{}
	ctx := newTestContext(t)

	headers := make(http.Header)
	p.Inject(ctx, headers)

	extracted, ok := p.Extract(headers)
	require.True(t, ok)
	assert.Equal(t, ctx.TraceID, extracted.TraceID)
	assert.Equal(t, ctx.SpanID, extracted.SpanID)
	assert.Equal(t, ctx.IsSampled, extracted.IsSampled)
	assert.True(t, extracted.IsRemote)
}

func TestW3C_SampledFlag(t *testing.T) {
	p := propagation.W3CPropagator{}

	// sampled=true → 01 in header
	ctx := newTestContext(t)
	ctx.IsSampled = true
	headers := make(http.Header)
	p.Inject(ctx, headers)
	assert.Contains(t, headers.Get("traceparent"), "-01")

	extracted, ok := p.Extract(headers)
	require.True(t, ok)
	assert.True(t, extracted.IsSampled)

	// sampled=false → 00 in header
	ctx.IsSampled = false
	headers = make(http.Header)
	p.Inject(ctx, headers)
	assert.Contains(t, headers.Get("traceparent"), "-00")

	extracted, ok = p.Extract(headers)
	require.True(t, ok)
	assert.False(t, extracted.IsSampled)
}

func TestW3C_InvalidTraceparent(t *testing.T) {
	p := propagation.W3CPropagator{}
	cases := []string{
		"",
		"bad",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // version != 00
		"00-tooshort-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-tooshort-01",
	}
	for _, val := range cases {
		headers := make(http.Header)
		headers.Set("traceparent", val)
		_, ok := p.Extract(headers)
		assert.False(t, ok, "expected false for traceparent %q", val)
	}
}

func TestW3C_MissingHeaders(t *testing.T) {
	p := propagation.W3CPropagator{}
	_, ok := p.Extract(make(http.Header))
	assert.False(t, ok)
}

func TestB3_InjectExtractRoundtrip(t *testing.T) {
	p := propagation.B3Propagator{}
	ctx := newTestContext(t)

	headers := make(http.Header)
	p.Inject(ctx, headers)

	extracted, ok := p.Extract(headers)
	require.True(t, ok)
	assert.Equal(t, ctx.TraceID, extracted.TraceID)
	assert.Equal(t, ctx.SpanID, extracted.SpanID)
	assert.Equal(t, ctx.IsSampled, extracted.IsSampled)
}

func TestB3Single_InjectExtractRoundtrip(t *testing.T) {
	p := propagation.B3Propagator{SingleHeader: true}
	ctx := newTestContext(t)

	headers := make(http.Header)
	p.Inject(ctx, headers)

	extracted, ok := p.Extract(headers)
	require.True(t, ok)
	assert.Equal(t, ctx.TraceID, extracted.TraceID)
	assert.Equal(t, ctx.SpanID, extracted.SpanID)
}

func TestComposite_W3CTakesPriority(t *testing.T) {
	comp := propagation.NewCompositePropagator()
	w3c := propagation.W3CPropagator{}
	b3 := propagation.B3Propagator{}

	w3cCtx := newTestContext(t)
	b3Ctx := newTestContext(t)

	headers := make(http.Header)
	w3c.Inject(w3cCtx, headers)
	b3.Inject(b3Ctx, headers)

	extracted, ok := comp.Extract(headers)
	require.True(t, ok)
	// Should use W3C (first priority)
	assert.Equal(t, w3cCtx.TraceID, extracted.TraceID)
}

func TestComposite_MissingHeaders(t *testing.T) {
	comp := propagation.NewCompositePropagator()
	_, ok := comp.Extract(make(http.Header))
	assert.False(t, ok)
}

func TestW3C_FuzzNoPanic(t *testing.T) {
	p := propagation.W3CPropagator{}
	cases := []string{
		"random-garbage",
		"00-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-yyyyyyyyyyyyyyyy-01",
		"00--00f067aa0ba902b7-01",
		string([]byte{0, 1, 2, 3, 4}),
	}
	for _, val := range cases {
		headers := make(http.Header)
		headers.Set("traceparent", val)
		// must not panic
		p.Extract(headers)
	}
}
