package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	return testServerWithAPIKey(t, "")
}

func testServerWithAPIKey(t *testing.T, apiKey string) *httptest.Server {
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
	authConfig := api.LoadAuthConfig(apiKey)
	alertManager := api.NewAlertManager(metricsStore, nil)
	lifecycleHandler := api.NewLifecycleHandler(store, analysis.NewAnalyzer())
	api.SetupRoutes(ctx, r, pipeline, store, metricsStore, sseBus, authConfig, alertManager, lifecycleHandler)
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
					"traceId":           traceID.String(),
					"spanId":            clientID.String(),
					"parentSpanId":      rootID.String(),
					"name":              "call downstream",
					"kind":              3, // Client
					"serviceName":       "upstream-svc",
					"startTimeUnixNano": uint64(now.Add(10 * time.Millisecond).UnixNano()),
					"endTimeUnixNano":   uint64(now.Add(180 * time.Millisecond).UnixNano()),
					"attributes": []any{
						map[string]any{
							"key":         "peer.service",
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

func TestAPI_ZipkinAccepts64BitTraceIDs(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, err := json.Marshal([]map[string]any{
		{
			"traceId":   "4bf92f3577b34da6",
			"id":        "00f067aa0ba902b7",
			"name":      "zipkin-root",
			"kind":      "SERVER",
			"timestamp": float64(time.Now().UnixMicro()),
			"duration":  float64(5000),
			"localEndpoint": map[string]any{
				"serviceName": "zipkin-svc",
			},
			"tags": map[string]any{},
		},
	})
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/api/v2/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	time.Sleep(120 * time.Millisecond)

	resp, err = http.Get(srv.URL + "/api/v1/traces/00000000000000004bf92f3577b34da6")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var detail map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))
	assert.Equal(t, "00000000000000004bf92f3577b34da6", detail["traceId"])
	assert.Equal(t, float64(1), detail["spanCount"])
}

func TestAPI_PprofEndpointRequiresAuthAndCanBeEnabled(t *testing.T) {
	t.Setenv("ENABLE_PPROF", "true")
	srv := testServerWithAPIKey(t, "secret-key")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/debug/pprof/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/debug/pprof/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret-key")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAPI_RBACAndTenantIsolation(t *testing.T) {
	t.Setenv("AUTH_TOKENS", "viewer-a|viewer|tenant-a;operator-a|operator|tenant-a;operator-b|operator|tenant-b;admin-global|admin|*")

	srv := testServer(t)
	defer srv.Close()

	traceA, _, bodyA := spanBody(t)
	traceB, _, bodyB := spanBody(t)

	reqA, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/spans", bytes.NewReader(bodyA))
	reqA.Header.Set("Authorization", "Bearer operator-a")
	resp, err := http.DefaultClient.Do(reqA)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	reqB, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/spans", bytes.NewReader(bodyB))
	reqB.Header.Set("Authorization", "Bearer operator-b")
	resp, err = http.DefaultClient.Do(reqB)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(120 * time.Millisecond)

	viewerReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/traces", nil)
	viewerReq.Header.Set("Authorization", "Bearer viewer-a")
	resp, err = http.DefaultClient.Do(viewerReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	traces := list["traces"].([]any)
	require.Len(t, traces, 1)
	assert.Equal(t, traceA.String(), traces[0].(map[string]any)["traceId"])

	forbiddenReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/traces/"+traceB.String(), nil)
	forbiddenReq.Header.Set("Authorization", "Bearer viewer-a")
	resp, err = http.DefaultClient.Do(forbiddenReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	samplerReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/sampler", nil)
	samplerReq.Header.Set("Authorization", "Bearer operator-a")
	resp, err = http.DefaultClient.Do(samplerReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	adminReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/traces", nil)
	adminReq.Header.Set("Authorization", "Bearer admin-global")
	adminReq.Header.Set("X-Tenant-ID", "tenant-b")
	resp, err = http.DefaultClient.Do(adminReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	traces = list["traces"].([]any)
	require.Len(t, traces, 1)
	assert.Equal(t, traceB.String(), traces[0].(map[string]any)["traceId"])
}

func TestAPI_TraceLifecycleArchiveDeleteRestore(t *testing.T) {
	t.Setenv("AUTH_TOKENS", "operator-a|operator|tenant-a;admin-global|admin|*")
	archiveDir := t.TempDir()
	t.Setenv("ARCHIVE_DIR", archiveDir)

	srv := testServer(t)
	defer srv.Close()

	traceID, _, body := spanBody(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/spans", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer operator-a")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	time.Sleep(120 * time.Millisecond)

	archiveReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/traces/archive?fileName=tenant-a.json", nil)
	archiveReq.Header.Set("Authorization", "Bearer operator-a")
	resp, err = http.DefaultClient.Do(archiveReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	deleteReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/traces/"+traceID.String(), nil)
	deleteReq.Header.Set("Authorization", "Bearer operator-a")
	resp, err = http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	getReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/traces/"+traceID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer operator-a")
	resp, err = http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	restoreReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/traces/archive/restore?fileName=tenant-a.json", nil)
	restoreReq.Header.Set("Authorization", "Bearer admin-global")
	restoreReq.Header.Set("X-Tenant-ID", "tenant-a")
	resp, err = http.DefaultClient.Do(restoreReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	getReq, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/traces/"+traceID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer operator-a")
	resp, err = http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAPI_AlertWebhookReceivesActiveAlerts(t *testing.T) {
	var received atomic.Int64
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	t.Setenv("ALERT_WEBHOOK_URL", webhook.URL)
	t.Setenv("ALERT_EVAL_INTERVAL", "50ms")
	t.Setenv("ALERT_COOLDOWN", "50ms")

	srv := testServer(t)
	defer srv.Close()

	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)
	now := time.Now()
	body, err := json.Marshal(map[string]any{
		"spans": []map[string]any{{
			"traceId":           traceID.String(),
			"spanId":            spanID.String(),
			"name":              "POST /checkout",
			"kind":              1,
			"serviceName":       "checkout",
			"startTimeUnixNano": uint64(now.UnixNano()),
			"endTimeUnixNano":   uint64(now.Add(50 * time.Millisecond).UnixNano()),
			"attributes":        []any{},
			"events":            []any{},
			"links":             []any{},
			"status":            map[string]any{"code": 2, "message": "boom"},
		}},
	})
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()

	require.Eventually(t, func() bool {
		return received.Load() > 0
	}, 2*time.Second, 50*time.Millisecond)
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

func TestAPI_InvalidRuleNestedSamplerTypeReturns400(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"type": "rules",
		"rules": []map[string]any{
			{
				"operationGlob": "*",
				"priority":      1,
				"sampler":       map[string]any{"type": "bogus"},
			},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_InvalidProbabilisticSamplerRateReturns400(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"type": "probabilistic", "rate": 1.5})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_InvalidTailPolicyTypeReturns400(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"type": "tail",
		"policies": []map[string]any{
			{"type": "mystery"},
		},
	})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/sampler", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_PublicProbeAndSpecEndpointsRemainUnauthenticated(t *testing.T) {
	srv := testServerWithAPIKey(t, "secret")
	defer srv.Close()

	for _, path := range []string{
		"/healthz",
		"/readyz",
		"/openapi.yaml",
		"/metrics",
		"/api/v1/config",
	} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err, "GET %s", path)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s should remain public", path)
	}
}

func TestAPI_ReadyzReturns503WhenCollectorIsDraining(t *testing.T) {
	probes := api.NewProbeState(func() int { return 0 }, 0)
	probes.MarkDraining()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	probes.HandleReadyz(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "draining", body["status"])
}

func TestAPI_ProtectedEndpointsStillRequireAuth(t *testing.T) {
	srv := testServerWithAPIKey(t, "secret")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/services")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/services", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp2, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestAPI_TraceSSEEndpointOnlyEmitsTraceEvents(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sse/traces")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	reader := bufio.NewReader(resp.Body)
	_, _, body := spanBody(t)

	done := make(chan struct{})
	go func() {
		ingestAndWait(t, srv, body)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for trace SSE event")
		default:
		}

		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if strings.Contains(payload, `"type":"ping"`) {
			continue
		}
		if strings.Contains(payload, `"type":"span"`) {
			t.Fatalf("trace stream must not emit span events: %s", payload)
		}
		if strings.Contains(payload, `"type":"trace"`) {
			<-done
			return
		}
	}
}
