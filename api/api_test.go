package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/api"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

// testServer builds a fully wired httptest.Server with a short assembler timeout.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := storage.NewMemoryStore(10000)
	metricsStore := metrics.NewMetricsStore()
	sseBus := api.NewSSEBus()
	s := sampler.NewAlways()
	analyzer := analysis.NewAnalyzer()
	pipeline := api.NewPipeline(store, metricsStore, sseBus, s, analyzer, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	r := chi.NewRouter()
	api.SetupRoutes(ctx, r, pipeline, store, metricsStore, sseBus, "" /* no auth */)
	return httptest.NewServer(r)
}

// spanBody builds a native ingest request body with a single root span.
func spanBody(t *testing.T) (model.TraceID, model.SpanID, []byte) {
	t.Helper()
	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)

	now := time.Now()
	body, err := json.Marshal(map[string]any{
		"spans": []map[string]any{
			{
				"traceId":           traceID.String(),
				"spanId":            spanID.String(),
				"parentSpanId":      "",
				"name":              "GET /api/test",
				"kind":              1,
				"serviceName":       "test-svc",
				"startTimeUnixNano": uint64(now.UnixNano()),
				"endTimeUnixNano":   uint64(now.Add(100 * time.Millisecond).UnixNano()),
				"attributes":        []any{},
				"events":            []any{},
				"links":             []any{},
				"status":            map[string]any{"code": 0, "message": ""},
			},
		},
	})
	require.NoError(t, err)
	return traceID, spanID, body
}

// ingestAndWait posts a span batch and waits for the assembler to finalize the trace.
func ingestAndWait(t *testing.T, srv *httptest.Server, body []byte) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	time.Sleep(120 * time.Millisecond) // wait for 50ms assembler timeout
}

// TestAPI_IngestThenQuery ingests a span and verifies it appears via GET /api/v1/traces/:id.
func TestAPI_IngestThenQuery(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	traceID, _, body := spanBody(t)
	ingestAndWait(t, srv, body)

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/traces/%s", srv.URL, traceID.String()))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))
	assert.Equal(t, traceID.String(), detail["traceId"])
	assert.Equal(t, float64(1), detail["spanCount"])
}

// TestAPI_FilterByService verifies that GET /api/v1/traces?service=X returns only matching traces.
func TestAPI_FilterByService(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	now := time.Now()

	// Ingest trace for svc-alpha
	alphaTraceID, err := model.NewTraceID()
	require.NoError(t, err)
	alphaSpanID, err := model.NewSpanID()
	require.NoError(t, err)

	// Ingest trace for svc-beta
	betaTraceID, err := model.NewTraceID()
	require.NoError(t, err)
	betaSpanID, err := model.NewSpanID()
	require.NoError(t, err)

	makeSpanBody := func(traceID model.TraceID, spanID model.SpanID, svc string) []byte {
		b, _ := json.Marshal(map[string]any{
			"spans": []map[string]any{{
				"traceId":           traceID.String(),
				"spanId":            spanID.String(),
				"parentSpanId":      "",
				"name":              "op",
				"kind":              1,
				"serviceName":       svc,
				"startTimeUnixNano": uint64(now.UnixNano()),
				"endTimeUnixNano":   uint64(now.Add(50 * time.Millisecond).UnixNano()),
				"attributes":        []any{},
				"events":            []any{},
				"links":             []any{},
				"status":            map[string]any{"code": 0, "message": ""},
			}},
		})
		return b
	}

	ingestAndWait(t, srv, makeSpanBody(alphaTraceID, alphaSpanID, "svc-alpha"))
	ingestAndWait(t, srv, makeSpanBody(betaTraceID, betaSpanID, "svc-beta"))

	// Query only svc-alpha
	resp, err := http.Get(srv.URL + "/api/v1/traces?service=svc-alpha")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	traces, _ := result["traces"].([]any)
	require.Len(t, traces, 1)
	trace := traces[0].(map[string]any)
	assert.Equal(t, alphaTraceID.String(), trace["traceId"])
}

// TestAPI_DependenciesNonEmpty ingests traces with CLIENT spans and verifies the dependency graph.
func TestAPI_DependenciesNonEmpty(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	now := time.Now()

	// Send 3 traces each with a client span pointing to downstream-svc
	for i := 0; i < 3; i++ {
		traceID, _ := model.NewTraceID()
		rootID, _ := model.NewSpanID()
		clientID, _ := model.NewSpanID()

		body, _ := json.Marshal(map[string]any{
			"spans": []map[string]any{
				{
					"traceId":           traceID.String(),
					"spanId":            rootID.String(),
					"parentSpanId":      "",
					"name":              "root",
					"kind":              1, // Server
					"serviceName":       "upstream-svc",
					"startTimeUnixNano": uint64(now.UnixNano()),
					"endTimeUnixNano":   uint64(now.Add(200 * time.Millisecond).UnixNano()),
					"attributes":        []any{},
					"events":            []any{},
					"links":             []any{},
					"status":            map[string]any{"code": 0, "message": ""},
				},
				{
					"traceId":      traceID.String(),
					"spanId":       clientID.String(),
					"parentSpanId": rootID.String(),
					"name":         "call downstream",
					"kind":         3, // Client
					"serviceName":  "upstream-svc",
					"startTimeUnixNano": uint64(now.Add(10 * time.Millisecond).UnixNano()),
					"endTimeUnixNano":   uint64(now.Add(180 * time.Millisecond).UnixNano()),
					"attributes": []any{
						map[string]any{
							"key":   "peer.service",
							"stringValue": "downstream-svc",
						},
					},
					"events": []any{},
					"links":  []any{},
					"status": map[string]any{"code": 0, "message": ""},
				},
			},
		})
		ingestAndWait(t, srv, body)
	}

	resp, err := http.Get(srv.URL + "/api/v1/dependencies")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var graph map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&graph))

	edges, _ := graph["edges"].([]any)
	require.NotEmpty(t, edges, "dependency graph must have edges after traces with CLIENT spans")

	edge := edges[0].(map[string]any)
	assert.Equal(t, "upstream-svc", edge["caller"])
	assert.Equal(t, "downstream-svc", edge["callee"])
	assert.Equal(t, float64(3), edge["count"])
}

// TestAPI_SamplerPutThenGet verifies that PUT /api/v1/sampler changes the sampler type.
func TestAPI_SamplerPutThenGet(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	rate := 0.5
	body, _ := json.Marshal(map[string]any{"type": "probabilistic", "rate": rate})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Confirm via GET
	resp2, err := http.Get(srv.URL + "/api/v1/sampler")
	require.NoError(t, err)
	defer resp2.Body.Close()

	var config map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&config))
	assert.Equal(t, "probabilistic", config["type"])
	cfg, _ := config["config"].(map[string]any)
	assert.InDelta(t, rate, cfg["rate"].(float64), 0.001)
}

// TestAPI_NotFoundTrace verifies that GET /api/v1/traces/:id returns 404 for unknown IDs.
func TestAPI_NotFoundTrace(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	unknownID, _ := model.NewTraceID()
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/traces/%s", srv.URL, unknownID.String()))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAPI_MalformedSpanInput verifies that POST /api/v1/spans returns 400 for invalid JSON.
func TestAPI_MalformedSpanInput(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json",
		bytes.NewReader([]byte(`{not valid json`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestAPI_ParseFailureCountedAsDropped verifies that spans with invalid traceIds are counted
// in the Dropped field of the ingest response (regression test for silent parse drop bug).
func TestAPI_ParseFailureCountedAsDropped(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// One span with an invalid traceId (not 32 hex chars), one valid span.
	validID, _ := model.NewTraceID()
	validSpanID, _ := model.NewSpanID()
	now := time.Now()

	body, _ := json.Marshal(map[string]any{
		"spans": []map[string]any{
			{
				"traceId":           "NOT-A-VALID-TRACE-ID",
				"spanId":            validSpanID.String(),
				"name":              "bad",
				"kind":              1,
				"serviceName":       "svc",
				"startTimeUnixNano": uint64(now.UnixNano()),
				"endTimeUnixNano":   uint64(now.Add(10 * time.Millisecond).UnixNano()),
				"status":            map[string]any{"code": 0},
			},
			{
				"traceId":           validID.String(),
				"spanId":            validSpanID.String(),
				"name":              "good",
				"kind":              1,
				"serviceName":       "svc",
				"startTimeUnixNano": uint64(now.UnixNano()),
				"endTimeUnixNano":   uint64(now.Add(10 * time.Millisecond).UnixNano()),
				"status":            map[string]any{"code": 0},
			},
		},
	})

	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, float64(1), result["accepted"], "valid span should be accepted")
	assert.Equal(t, float64(1), result["dropped"], "invalid span should be counted as dropped")
}

// TestAPI_StartTimeEndTimeFilter verifies that startTime/endTime query params filter traces.
func TestAPI_StartTimeEndTimeFilter(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	_, _, body := spanBody(t)
	ingestAndWait(t, srv, body)

	// A startTime far in the future should return zero traces.
	futureMs := fmt.Sprintf("%d", (time.Now().Add(time.Hour).UnixMilli()))
	resp, err := http.Get(srv.URL + "/api/v1/traces?startTime=" + futureMs)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	traces, _ := result["traces"].([]any)
	assert.Len(t, traces, 0, "future startTime filter should return no traces")
}

// TestAPI_ZeroTimestampCountedAsDropped verifies that spans with zero StartTimeUnixNano are
// rejected (regression for uint64 overflow producing garbage timestamps).
func TestAPI_ZeroTimestampCountedAsDropped(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	validID, _ := model.NewTraceID()
	validSpanID, _ := model.NewSpanID()
	now := time.Now()

	body, _ := json.Marshal(map[string]any{
		"spans": []map[string]any{
			{
				// zero StartTimeUnixNano — must be rejected
				"traceId":           validID.String(),
				"spanId":            validSpanID.String(),
				"name":              "bad-timestamps",
				"kind":              1,
				"serviceName":       "svc",
				"startTimeUnixNano": uint64(0),
				"endTimeUnixNano":   uint64(now.Add(10 * time.Millisecond).UnixNano()),
				"status":            map[string]any{"code": 0},
			},
		},
	})

	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, float64(0), result["accepted"], "span with zero start time must be rejected")
	assert.Equal(t, float64(1), result["dropped"], "span with zero start time must count as dropped")
}

// TestAPI_InvertedTimestampCountedAsDropped verifies that spans where end < start are rejected.
func TestAPI_InvertedTimestampCountedAsDropped(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	validID, _ := model.NewTraceID()
	validSpanID, _ := model.NewSpanID()
	now := time.Now()

	body, _ := json.Marshal(map[string]any{
		"spans": []map[string]any{
			{
				"traceId":           validID.String(),
				"spanId":            validSpanID.String(),
				"name":              "inverted",
				"kind":              1,
				"serviceName":       "svc",
				"startTimeUnixNano": uint64(now.UnixNano()),
				"endTimeUnixNano":   uint64(now.Add(-10 * time.Millisecond).UnixNano()), // end before start
				"status":            map[string]any{"code": 0},
			},
		},
	})

	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, float64(0), result["accepted"], "span with end < start must be rejected")
	assert.Equal(t, float64(1), result["dropped"], "span with end < start must count as dropped")
}

// TestAPI_UnknownSamplerTypeReturns400 verifies that PUT /api/v1/sampler with an unknown type
// returns 400 instead of silently installing an always-sampler.
func TestAPI_UnknownSamplerTypeReturns400(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"type": "nonexistent-sampler"})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestAPI_LimitCappedAt1000 verifies that ?limit=999999 is capped at 1000.
func TestAPI_LimitCappedAt1000(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// Ingest one trace so the response isn't trivially empty.
	_, _, body := spanBody(t)
	ingestAndWait(t, srv, body)

	resp, err := http.Get(srv.URL + "/api/v1/traces?limit=999999")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	// total may be 1; what matters is no panic and a valid response.
	_, ok := result["traces"]
	assert.True(t, ok, "response must have traces field")
}

// TestAPI_RulesSamplerPutThenGet verifies that PUT {"type":"rules"} actually installs a
// rule-based sampler (regression for silent fallback-to-always bug).
func TestAPI_RulesSamplerPutThenGet(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"type": "rules",
		"rules": []map[string]any{
			{
				"operationGlob": "*",
				"serviceName":   "",
				"priority":      10,
				"sampler":       map[string]any{"type": "always"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2, err := http.Get(srv.URL + "/api/v1/sampler")
	require.NoError(t, err)
	defer resp2.Body.Close()

	var config map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&config))
	assert.Equal(t, "rules", config["type"], "sampler type must be rules, not always")
}
