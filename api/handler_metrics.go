package api

import (
	"net/http"

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
	snapshots := h.metricsStore.Snapshot()
	writeJSON(w, map[string]any{"metrics": snapshots})
}

// HandleHeatmap handles GET /api/v1/metrics/heatmap?service=X&window=3600
func (h *MetricsHandler) HandleHeatmap(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	buckets := h.metricsStore.HeatmapData(service)
	writeJSON(w, map[string]any{
		"resolution": "10s",
		"buckets":    buckets,
	})
}
