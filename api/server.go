package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

func SetupRoutes(ctx context.Context, r *chi.Mux, pipeline *Pipeline, store storage.TraceStore,
	metricsStore *metrics.MetricsStore, sseBus *SSEBus, apiKey string) {

	// Wire pipeline worker pool shutdown to context
	pipeline.StartWithContext(ctx)

	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "application/json", "text/plain", "text/html"))
	r.Use(CORS)
	r.Use(APIKeyAuth(apiKey))

	// Health probes and API spec (unauthenticated)
	r.Get("/healthz", HandleHealthz)
	r.Get("/readyz", HandleReadyz)
	r.Get("/openapi.yaml", HandleOpenAPI)

	// Prometheus self-observability metrics
	r.Get("/metrics", NewPrometheusHandler(metricsStore, pipeline).ServeHTTP)

	// Frontend configuration
	r.Get("/api/v1/config", HandleConfig)

	// Collector stats
	r.Get("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		sampled, dropped := pipeline.Stats()
		storeStats := store.Stats()
		writeJSON(w, map[string]any{
			"sampledTotal":    sampled,
			"droppedTotal":    dropped,
			"workerQueueDepth": pipeline.QueueDepth(),
			"traceCount":      storeStats.TraceCount,
			"maxTraces":       storeStats.MaxSize,
		})
	})

	ingest := NewIngestHandler(pipeline)
	query := NewQueryHandler(store, pipeline)
	metricsH := NewMetricsHandler(metricsStore)
	samplerH := NewSamplerHandler(pipeline)

	// Ingest
	r.Post("/api/v1/spans", ingest.HandleNativeSpans)
	r.Post("/v1/traces", ingest.HandleOTLPTraces)
	r.Post("/api/v2/spans", ingest.HandleZipkinSpans) // Zipkin v2 JSON

	// Query
	r.Get("/api/v1/traces", query.HandleListTraces)
	r.Get("/api/v1/traces/compare", query.HandleCompareTraces)
	r.Get("/api/v1/traces/{traceId}", query.HandleGetTrace)
	r.Get("/api/v1/traces/{traceId}/export", query.HandleExportTrace)
	r.Get("/api/v1/services", query.HandleGetServices)
	r.Get("/api/v1/operations", query.HandleGetOperations)
	r.Get("/api/v1/dependencies", query.HandleGetDependencies)

	// Metrics
	r.Get("/api/v1/metrics/red", metricsH.HandleREDMetrics)
	r.Get("/api/v1/metrics/heatmap", metricsH.HandleHeatmap)
	r.Get("/api/v1/metrics/anomalies", metricsH.HandleAnomalies)
	r.Get("/api/v1/metrics/slo", metricsH.HandleSLOs)

	// Sampler
	r.Get("/api/v1/sampler", samplerH.HandleGetSampler)
	r.Put("/api/v1/sampler", samplerH.HandlePutSampler)

	// SSE — separate buses per stream type so each endpoint only receives its own events.
	// sseBus receives span + trace events from the pipeline.
	metricsBus := NewSSEBus()
	samplerBus := NewSSEBus()

	r.Get("/sse/spans", sseBus.ServeSSE)
	r.Get("/sse/traces", sseBus.ServeSSE)
	r.Get("/sse/metrics", metricsBus.ServeSSE)
	r.Get("/sse/sampler", samplerBus.ServeSSE)

	// Broadcast a metrics tick every second so the Metrics page refreshes live.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				metricsBus.Broadcast(SSEEvent{Type: "metrics"})
			case <-ctx.Done():
				return
			}
		}
	}()

	// Broadcast a sampler tick every 5 seconds so the Sampler page refreshes live.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sampledTotal, droppedTotal := pipeline.Stats()
				total := sampledTotal + droppedTotal
				var rate float64
				if total > 0 {
					rate = float64(sampledTotal) / float64(total)
				}
				samplerBus.Broadcast(SSEEvent{Type: "sampler", Data: map[string]any{
					"samplingRate": rate,
					"sampledTotal": sampledTotal,
					"droppedTotal": droppedTotal,
				}})
			case <-ctx.Done():
				return
			}
		}
	}()
}

// NewPipelineWithDefaults creates a pipeline with sensible defaults.
func NewPipelineWithDefaults(store storage.TraceStore, metricsStore *metrics.MetricsStore,
	sseBus *SSEBus) *Pipeline {
	s := sampler.NewAlways()
	analyzer := analysis.NewAnalyzer()
	return NewPipeline(store, metricsStore, sseBus, s, analyzer)
}

