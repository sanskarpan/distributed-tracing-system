package api

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

func SetupRoutes(ctx context.Context, r *chi.Mux, pipeline *Pipeline, store storage.TraceStore,
	metricsStore *metrics.MetricsStore, sseBus *SSEBus, authConfig AuthConfig,
	alertManager *AlertManager, lifecycle *LifecycleHandler) *ProbeState {

	// Wire pipeline worker pool shutdown to context
	pipeline.StartWithContext(ctx)
	probes := NewProbeState(pipeline.QueueDepth, pipeline.QueueCapacity())
	if alertManager != nil {
		alertManager.SetProbes(probes)
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "application/json", "text/plain", "text/html"))
	r.Use(CORS)

	// Public endpoints
	r.Get("/healthz", probes.HandleHealthz)
	r.Get("/readyz", probes.HandleReadyz)
	r.Get("/openapi.yaml", HandleOpenAPI)
	r.Get("/metrics", NewPrometheusHandler(metricsStore, pipeline).ServeHTTP)
	r.Get("/api/v1/config", HandleConfig)

	ingest := NewIngestHandler(pipeline)
	query := NewQueryHandler(store, pipeline)
	metricsH := NewMetricsHandler(metricsStore)
	samplerH := NewSamplerHandler(pipeline)

	if alertManager != nil {
		alertManager.Start(ctx)
	}

	// SSE — separate buses per stream type so each endpoint only receives its own events.
	// sseBus receives span + trace events from the pipeline.
	metricsBus := NewSSEBus()
	samplerBus := NewSSEBus()

	r.Group(func(protected chi.Router) {
		protected.Use(AuthMiddleware(authConfig))

		protected.Group(func(viewer chi.Router) {
			viewer.Use(RequireRole(RoleViewer))

			// Query
			viewer.Get("/api/v1/traces", query.HandleListTraces)
			viewer.Get("/api/v1/traces/compare", query.HandleCompareTraces)
			viewer.Get("/api/v1/traces/{traceId}", query.HandleGetTrace)
			viewer.Get("/api/v1/traces/{traceId}/export", query.HandleExportTrace)
			viewer.Get("/api/v1/services", query.HandleGetServices)
			viewer.Get("/api/v1/operations", query.HandleGetOperations)
			viewer.Get("/api/v1/dependencies", query.HandleGetDependencies)

			// Metrics
			viewer.Get("/api/v1/metrics/red", metricsH.HandleREDMetrics)
			viewer.Get("/api/v1/metrics/heatmap", metricsH.HandleHeatmap)
			viewer.Get("/api/v1/metrics/anomalies", metricsH.HandleAnomalies)
			viewer.Get("/api/v1/metrics/slo", metricsH.HandleSLOs)

			// Alerts
			if alertManager != nil {
				viewer.Get("/api/v1/alerts", alertManager.HandleGetAlerts)
			}

			// SSE
			viewer.Get("/sse/spans", func(w http.ResponseWriter, r *http.Request) {
				sseBus.ServeFilteredSSE(w, r, "span")
			})
			viewer.Get("/sse/traces", func(w http.ResponseWriter, r *http.Request) {
				sseBus.ServeFilteredSSE(w, r, "trace")
			})
			viewer.Get("/sse/metrics", metricsBus.ServeSSE)
			viewer.Get("/sse/sampler", samplerBus.ServeSSE)
		})

		protected.Group(func(operator chi.Router) {
			operator.Use(RequireRole(RoleOperator))

			// Ingest
			operator.Post("/api/v1/spans", ingest.HandleNativeSpans)
			operator.Post("/v1/traces", ingest.HandleOTLPTraces)
			operator.Post("/api/v2/spans", ingest.HandleZipkinSpans) // Zipkin v2 JSON

			if lifecycle != nil {
				operator.Post("/api/v1/traces/import", lifecycle.HandleImportTrace)
				operator.Delete("/api/v1/traces/{traceId}", lifecycle.HandleDeleteTrace)
				operator.Post("/api/v1/traces/archive", lifecycle.HandleArchiveSnapshot)
			}
		})

		protected.Group(func(admin chi.Router) {
			admin.Use(RequireRole(RoleAdmin))

			// Collector stats
			admin.Get("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
				sampled, dropped := pipeline.Stats()
				storeStats := store.Stats()
				writeJSON(w, map[string]any{
					"sampledTotal":     sampled,
					"droppedTotal":     dropped,
					"workerQueueDepth": pipeline.QueueDepth(),
					"traceCount":       storeStats.TraceCount,
					"maxTraces":        storeStats.MaxSize,
				})
			})

			// Sampler
			admin.Get("/api/v1/sampler", samplerH.HandleGetSampler)
			admin.Put("/api/v1/sampler", samplerH.HandlePutSampler)

			if lifecycle != nil {
				admin.Post("/api/v1/traces/archive/restore", lifecycle.HandleRestoreSnapshot)
			}

			if os.Getenv("ENABLE_PPROF") == "true" {
				admin.HandleFunc("/debug/pprof/", pprof.Index)
				admin.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
				admin.HandleFunc("/debug/pprof/profile", pprof.Profile)
				admin.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
				admin.HandleFunc("/debug/pprof/trace", pprof.Trace)
				admin.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
				admin.Handle("/debug/pprof/block", pprof.Handler("block"))
				admin.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
				admin.Handle("/debug/pprof/heap", pprof.Handler("heap"))
				admin.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
				admin.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
			}
		})
	})

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

	return probes
}

// NewPipelineWithDefaults creates a pipeline with sensible defaults.
func NewPipelineWithDefaults(store storage.TraceStore, metricsStore *metrics.MetricsStore,
	sseBus *SSEBus) *Pipeline {
	s := sampler.NewAlways()
	analyzer := analysis.NewAnalyzer()
	return NewPipeline(store, metricsStore, sseBus, s, analyzer)
}
