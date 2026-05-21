package api_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/tracing/api"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

func newFuzzServer() (srv *httptest.Server, cancel context.CancelFunc) {
	store := storage.NewMemoryStore(1000)
	ms := metrics.NewMetricsStore()
	bus := api.NewSSEBus()
	s := sampler.NewAlways()
	analyzer := analysis.NewAnalyzer()
	pipeline := api.NewPipeline(store, ms, bus, s, analyzer, 20*time.Millisecond)
	r := chi.NewRouter()
	ctx, cancel := context.WithCancel(context.Background())
	alertManager := api.NewAlertManager(ms, nil)
	lifecycleHandler := api.NewLifecycleHandler(store, analysis.NewAnalyzer())
	api.SetupRoutes(ctx, r, pipeline, store, ms, bus, api.LoadAuthConfig(""), nil, alertManager, lifecycleHandler)
	return httptest.NewServer(r), cancel
}

// FuzzIngestSpans fuzzes the JSON body of POST /api/v1/spans.
// Verifies: no panics, only 202/400 responses (never 5xx).
func FuzzIngestSpans(f *testing.F) {
	traceID, _ := model.NewTraceID()
	spanID, _ := model.NewSpanID()
	now := uint64(time.Now().UnixNano())
	seed := fmt.Sprintf(`{"spans":[{"traceId":%q,"spanId":%q,"name":"op","serviceName":"svc","startTimeUnixNano":%d,"endTimeUnixNano":%d,"status":{"code":0}}]}`,
		traceID.String(), spanID.String(), now, now+1000000)
	f.Add([]byte(seed))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"spans":[]}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"spans":[{}]}`))

	srv, cancel := newFuzzServer()
	f.Cleanup(func() { cancel(); srv.Close() })

	f.Fuzz(func(t *testing.T, body []byte) {
		resp, err := http.Post(srv.URL+"/api/v1/spans", "application/json", bytes.NewReader(body))
		if err != nil {
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			t.Errorf("unexpected 5xx status %d for body %q", resp.StatusCode, body)
		}
	})
}

// FuzzQueryParams fuzzes query parameters to GET /api/v1/traces.
// Verifies: no panics, no 5xx responses.
func FuzzQueryParams(f *testing.F) {
	f.Add("service", "op", "20", "0", "100", "1000", "error", "receivedAt")
	f.Add("", "", "-1", "abc", "9999999999", "0", "all", "")
	f.Add("'; DROP TABLE--", "", "1", "0", "", "", "", "")

	srv, cancel := newFuzzServer()
	f.Cleanup(func() { cancel(); srv.Close() })

	f.Fuzz(func(t *testing.T, service, operation, limit, offset, minDur, maxDur, status, sortBy string) {
		url := fmt.Sprintf("%s/api/v1/traces?service=%s&operation=%s&limit=%s&offset=%s&minDuration=%s&maxDuration=%s&status=%s&sortBy=%s",
			srv.URL, service, operation, limit, offset, minDur, maxDur, status, sortBy)
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			t.Errorf("unexpected 5xx for query params, status %d", resp.StatusCode)
		}
	})
}
