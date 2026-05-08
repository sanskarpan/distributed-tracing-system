package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/storage"
)

type QueryHandler struct {
	store    storage.TraceStore
	pipeline *Pipeline
}

func NewQueryHandler(store storage.TraceStore, pipeline *Pipeline) *QueryHandler {
	return &QueryHandler{store: store, pipeline: pipeline}
}

// HandleListTraces handles GET /api/v1/traces
func (h *QueryHandler) HandleListTraces(w http.ResponseWriter, r *http.Request) {
	q := buildTraceQuery(r)
	result, err := h.store.Query(q)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, 500)
		return
	}

	summaries := make([]TraceSummaryDTO, 0, len(result.Traces))
	for _, ts := range result.Traces {
		summaries = append(summaries, TraceSummaryDTO{
			TraceID:     ts.TraceID.String(),
			RootService: ts.RootService,
			RootOp:      ts.RootOp,
			DurationMs:  float64(ts.Duration.Nanoseconds()) / 1e6,
			SpanCount:   ts.SpanCount,
			Services:    ts.Services,
			HasError:    ts.HasError,
			ReceivedAt:  ts.ReceivedAt,
		})
	}

	writeJSON(w, map[string]any{
		"traces":  summaries,
		"total":   result.Total,
		"hasMore": result.HasMore,
	})
}

// HandleGetTrace handles GET /api/v1/traces/:traceId
func (h *QueryHandler) HandleGetTrace(w http.ResponseWriter, r *http.Request) {
	traceIDStr := chi.URLParam(r, "traceId")
	traceID, err := model.ParseTraceID(traceIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid trace ID"}`, 400)
		return
	}

	trace, ok := h.store.Get(traceID)
	if !ok {
		http.Error(w, `{"error":"trace not found"}`, 404)
		return
	}

	writeJSON(w, traceToDetailDTO(trace))
}

// HandleGetServices handles GET /api/v1/services
func (h *QueryHandler) HandleGetServices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"services": h.store.Services()})
}

// HandleGetOperations handles GET /api/v1/operations?service=X
func (h *QueryHandler) HandleGetOperations(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	writeJSON(w, map[string]any{"operations": h.store.Operations(service)})
}

// HandleGetDependencies handles GET /api/v1/dependencies
func (h *QueryHandler) HandleGetDependencies(w http.ResponseWriter, r *http.Request) {
	// Get all recent traces
	q := &storage.TraceQuery{Limit: 1000, SortBy: "receivedAt", SortDesc: true}
	result, err := h.store.Query(q)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, 500)
		return
	}

	traces := make([]*model.Trace, 0, len(result.Traces))
	for _, ts := range result.Traces {
		if t, ok := h.store.Get(ts.TraceID); ok {
			traces = append(traces, t)
		}
	}

	graph := h.pipeline.analyzer.BuildDependencyGraph(traces, time.Now().Add(-1*time.Hour))

	writeJSON(w, map[string]any{
		"services": graph.Services,
		"edges":    graph.Edges,
	})
}

// HandleExportTrace handles GET /api/v1/traces/{traceId}/export
// It returns the full trace as a downloadable JSON file.
func (h *QueryHandler) HandleExportTrace(w http.ResponseWriter, r *http.Request) {
	traceIDStr := chi.URLParam(r, "traceId")
	traceID, err := model.ParseTraceID(traceIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid trace ID"}`, 400)
		return
	}

	trace, ok := h.store.Get(traceID)
	if !ok {
		http.Error(w, `{"error":"trace not found"}`, 404)
		return
	}

	filename := "trace-" + traceIDStr + ".json"
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	writeJSON(w, traceToDetailDTO(trace))
}

// HandleCompareTraces handles GET /api/v1/traces/compare?base=X&compare=Y
func (h *QueryHandler) HandleCompareTraces(w http.ResponseWriter, r *http.Request) {
	baseIDStr := r.URL.Query().Get("base")
	compareIDStr := r.URL.Query().Get("compare")

	baseID, err1 := model.ParseTraceID(baseIDStr)
	compareID, err2 := model.ParseTraceID(compareIDStr)
	if err1 != nil || err2 != nil {
		http.Error(w, `{"error":"invalid trace IDs"}`, 400)
		return
	}

	base, ok1 := h.store.Get(baseID)
	compare, ok2 := h.store.Get(compareID)
	if !ok1 || !ok2 {
		http.Error(w, `{"error":"trace not found"}`, 404)
		return
	}

	result := h.pipeline.analyzer.CompareTraces(base, compare)

	matched := make([]map[string]any, 0, len(result.Matched))
	for _, m := range result.Matched {
		matched = append(matched, map[string]any{
			"baseSpanId":      m.BaseSpanID.String(),
			"compareSpanId":   m.CompareSpanID.String(),
			"durationDeltaMs": m.DurationDeltaMs,
		})
	}

	onlyInBase := make([]string, 0, len(result.OnlyInBase))
	for _, id := range result.OnlyInBase {
		onlyInBase = append(onlyInBase, id.String())
	}

	onlyInCompare := make([]string, 0, len(result.OnlyInCompare))
	for _, id := range result.OnlyInCompare {
		onlyInCompare = append(onlyInCompare, id.String())
	}

	writeJSON(w, map[string]any{
		"durationDeltaMs": result.DurationDeltaMs,
		"spanCountDelta":  result.SpanCountDelta,
		"errorDelta":      result.ErrorDelta,
		"matched":         matched,
		"onlyInBase":      onlyInBase,
		"onlyInCompare":   onlyInCompare,
	})
}

func buildTraceQuery(r *http.Request) *storage.TraceQuery {
	q := &storage.TraceQuery{
		ServiceName:   r.URL.Query().Get("service"),
		OperationName: r.URL.Query().Get("operation"),
		AttributeKV:   r.URL.Query().Get("attr"),
		SortBy:        r.URL.Query().Get("sortBy"),
		SortDesc:      r.URL.Query().Get("sortDesc") == "true",
		Limit:         20,
	}

	if lim, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && lim > 0 {
		if lim > 1000 {
			lim = 1000
		}
		q.Limit = lim
	}
	if off, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil {
		q.Offset = off
	}

	if minDur, err := strconv.ParseInt(r.URL.Query().Get("minDuration"), 10, 64); err == nil {
		d := time.Duration(minDur) * time.Millisecond
		q.MinDuration = &d
	}
	if maxDur, err := strconv.ParseInt(r.URL.Query().Get("maxDuration"), 10, 64); err == nil {
		d := time.Duration(maxDur) * time.Millisecond
		q.MaxDuration = &d
	}

	if status := r.URL.Query().Get("status"); status == "error" {
		hasError := true
		q.HasError = &hasError
	}

	if st, err := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64); err == nil {
		t := time.UnixMilli(st)
		q.StartTime = &t
	}
	if et, err := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64); err == nil {
		t := time.UnixMilli(et)
		q.EndTime = &t
	}

	if q.SortBy == "" {
		q.SortBy = "receivedAt"
		q.SortDesc = true
	}

	return q
}

func traceToDetailDTO(trace *model.Trace) TraceDetailDTO {
	var traceStartNano uint64
	for _, sp := range trace.Spans {
		if ns := uint64(sp.StartTime.UnixNano()); ns > 0 {
			if traceStartNano == 0 || ns < traceStartNano {
				traceStartNano = ns
			}
		}
	}

	spans := make([]SpanDetailDTO, 0, len(trace.Spans))
	for _, sp := range trace.Spans {
		spans = append(spans, spanToDetailDTO(sp, traceStartNano))
	}

	criticalPath := make([]string, 0, len(trace.CriticalPath))
	for _, sp := range trace.CriticalPath {
		criticalPath = append(criticalPath, sp.SpanID.String())
	}

	groups := make([]ParallelGroupDTO, 0, len(trace.ParallelGroups))
	for _, pg := range trace.ParallelGroups {
		ids := make([]string, 0, len(pg.Spans))
		for _, sp := range pg.Spans {
			ids = append(ids, sp.SpanID.String())
		}
		startMs := float64(pg.StartTime.UnixNano()-int64(traceStartNano)) / 1e6
		endMs := float64(pg.EndTime.UnixNano()-int64(traceStartNano)) / 1e6
		groups = append(groups, ParallelGroupDTO{SpanIDs: ids, StartMs: startMs, EndMs: endMs})
	}

	gaps := make([]SpanGapDTO, 0, len(trace.Gaps))
	for _, g := range trace.Gaps {
		beforeID := ""
		if g.Before != nil {
			beforeID = g.Before.SpanID.String()
		}
		afterID := ""
		if g.After != nil {
			afterID = g.After.SpanID.String()
		}
		gaps = append(gaps, SpanGapDTO{
			BeforeSpanID: beforeID,
			AfterSpanID:  afterID,
			DurationMs:   float64(g.Duration.Nanoseconds()) / 1e6,
		})
	}

	return TraceDetailDTO{
		TraceID:        trace.TraceID.String(),
		Spans:          spans,
		CriticalPath:   criticalPath,
		Services:       trace.Services,
		DurationMs:     float64(trace.Duration.Nanoseconds()) / 1e6,
		SpanCount:      trace.SpanCount,
		ErrorCount:     trace.ErrorCount,
		ParallelGroups: groups,
		Gaps:           gaps,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
