package api

import (
	"net/http"
	"strconv"

	"github.com/yourname/tracing/internal/metrics"
)

type MetricsHandler struct {
	metricsStore *metrics.MetricsStore
}

func NewMetricsHandler(m *metrics.MetricsStore) *MetricsHandler {
	return &MetricsHandler{metricsStore: m}
}

// HandleREDMetrics handles GET /api/v1/metrics/red
func (h *MetricsHandler) HandleREDMetrics(w http.ResponseWriter, r *http.Request) {
	snapshots := h.metricsStore.Snapshot(EffectiveTenant(PrincipalFromContext(r.Context())))
	writeJSON(w, map[string]any{"metrics": snapshots})
}

// HandleHeatmap handles GET /api/v1/metrics/heatmap?service=X
// Returns both span-count time-series and a 2D latency × time heatmap.
func (h *MetricsHandler) HandleHeatmap(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	tenantID := EffectiveTenant(PrincipalFromContext(r.Context()))
	buckets := h.metricsStore.HeatmapData(service, tenantID)
	latency := h.metricsStore.LatencyHeatmap2D(service, tenantID)
	writeJSON(w, map[string]any{
		"resolution": "10s",
		"buckets":    buckets,
		"latency":    latency,
	})
}

// HandleSLOs handles GET /api/v1/metrics/slo?target=0.01
// Returns error budget status per service against the target error rate.
func (h *MetricsHandler) HandleSLOs(w http.ResponseWriter, r *http.Request) {
	target := 0.01 // default: 99% availability SLO
	if v := r.URL.Query().Get("target"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 && parsed <= 1 {
			target = parsed
		}
	}
	results := h.metricsStore.ComputeSLOs(target, EffectiveTenant(PrincipalFromContext(r.Context())))
	writeJSON(w, map[string]any{"slos": results, "targetErrorRate": target})
}

// HandleAnomalies handles GET /api/v1/metrics/anomalies?z=2.0
// Returns operations whose P99 latency is more than z standard deviations above the mean.
func (h *MetricsHandler) HandleAnomalies(w http.ResponseWriter, r *http.Request) {
	z := 2.0
	if v := r.URL.Query().Get("z"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			z = parsed
		}
	}
	results := h.metricsStore.DetectAnomalies(z, EffectiveTenant(PrincipalFromContext(r.Context())))
	writeJSON(w, map[string]any{"anomalies": results, "zThreshold": z})
}
