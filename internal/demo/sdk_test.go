package demo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/demo"
	"github.com/yourname/tracing/internal/model"
)

func TestDemoSDK_StartFinishSpan(t *testing.T) {
	sdk := demo.NewDemoSDK("test-svc", "http://localhost:9999")
	ctx := context.Background()

	ctx, span := sdk.StartSpan(ctx, "test op", model.SpanKindServer)
	require.NotNil(t, span)
	assert.False(t, span.SpanID.IsZero())
	assert.False(t, span.TraceID.IsZero())
	assert.Equal(t, "test-svc", span.ServiceName)
	assert.False(t, span.StartTime.IsZero())
	assert.True(t, span.EndTime.IsZero(), "EndTime should be zero before Finish")

	sdk.FinishSpan(span)
	assert.False(t, span.EndTime.IsZero())
	assert.True(t, !span.EndTime.Before(span.StartTime))
	_ = ctx
}

func TestDemoSDK_NestedSpans(t *testing.T) {
	sdk := demo.NewDemoSDK("test-svc", "http://localhost:9999")
	ctx := context.Background()

	ctx, parent := sdk.StartSpan(ctx, "parent", model.SpanKindServer)
	_, child := sdk.StartSpan(ctx, "child", model.SpanKindInternal)

	assert.Equal(t, parent.SpanID, child.ParentSpanID)
	assert.Equal(t, parent.TraceID, child.TraceID)
}

func TestDemoSDK_InjectExtractRoundtrip(t *testing.T) {
	sdk := demo.NewDemoSDK("svc-a", "http://localhost:9999")
	ctx := context.Background()
	ctx, span := sdk.StartSpan(ctx, "root", model.SpanKindServer)

	headers := make(http.Header)
	sdk.InjectHTTP(ctx, headers)
	assert.NotEmpty(t, headers.Get("traceparent"))

	sdk2 := demo.NewDemoSDK("svc-b", "http://localhost:9999")
	ctx2 := sdk2.ExtractHTTP(context.Background(), headers)
	_, child := sdk2.StartSpan(ctx2, "child", model.SpanKindServer)

	assert.Equal(t, span.TraceID, child.TraceID)
}

func TestScenario_SuccessfulCheckout(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/spans" {
			var body struct {
				Spans []map[string]any `json:"spans"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				mu.Lock()
				received = append(received, body.Spans...)
				mu.Unlock()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"accepted":1}`))
	}))
	defer server.Close()

	sdk := demo.NewDemoSDK("frontend-svc", server.URL)
	scenario := demo.Scenarios[0]
	assert.Equal(t, "successful_checkout", scenario.Name)

	err := scenario.Run(sdk)
	require.NoError(t, err)

	mu.Lock()
	spanCount := len(received)
	mu.Unlock()

	// successful_checkout generates 11 spans:
	// frontend(1) + api-gateway(3: root+inv-client+pay-client) +
	// inventory-svc(3: server+cache+db) + payment-svc(4: server+redis+stripe+db-insert)
	assert.Equal(t, 11, spanCount, "successful_checkout must produce exactly 11 spans")

	// All spans must share the same traceId
	assert.Greater(t, spanCount, 0)
	if spanCount > 0 {
		traceID := received[0]["traceId"]
		for _, s := range received[1:] {
			assert.Equal(t, traceID, s["traceId"], "all spans must share the same traceId")
		}
	}

	// Verify expected service names are present
	services := make(map[string]bool)
	for _, s := range received {
		if svc, ok := s["serviceName"].(string); ok {
			services[svc] = true
		}
	}
	assert.True(t, services["frontend-svc"], "frontend-svc spans expected")
	assert.True(t, services["api-gateway"], "api-gateway spans expected")
	assert.True(t, services["inventory-svc"], "inventory-svc spans expected")
	assert.True(t, services["payment-svc"], "payment-svc spans expected")
}

func TestDemoSDK_Export(t *testing.T) {
	var received []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/spans" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if spans, ok := body["spans"].([]any); ok {
				for _, s := range spans {
					if sp, ok := s.(map[string]any); ok {
						received = append(received, sp)
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"accepted":1}`))
	}))
	defer server.Close()

	sdk := demo.NewDemoSDK("test-svc", server.URL)
	ctx := context.Background()
	ctx, span := sdk.StartSpan(ctx, "test", model.SpanKindServer)
	time.Sleep(1 * time.Millisecond)
	sdk.FinishSpan(span)

	err := sdk.Export()
	require.NoError(t, err)
	assert.Len(t, received, 1)
	assert.Equal(t, span.TraceID.String(), received[0]["traceId"])
	_ = ctx
}
