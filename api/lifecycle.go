package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/storage"
)

type LifecycleHandler struct {
	store      storage.TraceStore
	analyzer   *analysis.Analyzer
	archiveDir string
}

type archivePayload struct {
	ExportedAt time.Time        `json:"exportedAt"`
	TenantID   string           `json:"tenantId,omitempty"`
	TraceCount int              `json:"traceCount"`
	Traces     []TraceDetailDTO `json:"traces"`
}

func NewLifecycleHandler(store storage.TraceStore, analyzer *analysis.Analyzer) *LifecycleHandler {
	return &LifecycleHandler{
		store:      store,
		analyzer:   analyzer,
		archiveDir: strings.TrimSpace(os.Getenv("ARCHIVE_DIR")),
	}
}

func (h *LifecycleHandler) HandleImportTrace(w http.ResponseWriter, r *http.Request) {
	principal := PrincipalFromContext(r.Context())
	tenantID := EffectiveTenant(principal)

	var payload struct {
		Trace  *TraceDetailDTO  `json:"trace"`
		Traces []TraceDetailDTO `json:"traces"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	var imported int
	if payload.Trace != nil {
		if err := h.importTrace(principal, tenantID, *payload.Trace); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		imported++
	}
	for _, trace := range payload.Traces {
		if err := h.importTrace(principal, tenantID, trace); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		imported++
	}

	writeJSON(w, map[string]any{"ok": true, "imported": imported})
}

func (h *LifecycleHandler) HandleDeleteTrace(w http.ResponseWriter, r *http.Request) {
	traceID, err := model.ParseTraceID(chi.URLParam(r, "traceId"))
	if err != nil {
		http.Error(w, `{"error":"invalid trace ID"}`, http.StatusBadRequest)
		return
	}

	trace, ok := h.store.Get(traceID)
	if !ok || !principalCanAccessTenant(PrincipalFromContext(r.Context()), trace.TenantID) {
		http.Error(w, `{"error":"trace not found"}`, http.StatusNotFound)
		return
	}
	if err := h.store.Delete(traceID); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "traceId": traceID.String()})
}

func (h *LifecycleHandler) HandleArchiveSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.archiveDir == "" {
		http.Error(w, `{"error":"ARCHIVE_DIR is not configured"}`, http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(h.archiveDir, 0o755); err != nil {
		http.Error(w, `{"error":"archive directory unavailable"}`, http.StatusInternalServerError)
		return
	}

	principal := PrincipalFromContext(r.Context())
	tenantID := EffectiveTenant(principal)
	traces, err := h.store.List(&storage.TraceQuery{
		TenantID: tenantID,
		Limit:    100000,
		SortBy:   "receivedAt",
		SortDesc: true,
	})
	if err != nil {
		http.Error(w, `{"error":"archive query failed"}`, http.StatusInternalServerError)
		return
	}

	details := make([]TraceDetailDTO, 0, len(traces))
	for _, trace := range traces {
		details = append(details, traceToDetailDTO(trace))
	}

	fileName := time.Now().UTC().Format("20060102-150405") + "-traces.json"
	if requested := sanitizeArchiveName(r.URL.Query().Get("fileName")); requested != "" {
		fileName = requested
	}
	fullPath := filepath.Join(h.archiveDir, fileName)
	file, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, `{"error":"archive file could not be created"}`, http.StatusInternalServerError)
		return
	}
	defer file.Close()

	payload := archivePayload{
		ExportedAt: time.Now().UTC(),
		TenantID:   tenantID,
		TraceCount: len(details),
		Traces:     details,
	}
	if err := json.NewEncoder(file).Encode(payload); err != nil {
		http.Error(w, `{"error":"archive write failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"ok": true, "fileName": fileName, "traceCount": len(details)})
}

func (h *LifecycleHandler) HandleRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	if h.archiveDir == "" {
		http.Error(w, `{"error":"ARCHIVE_DIR is not configured"}`, http.StatusBadRequest)
		return
	}
	fileName := sanitizeArchiveName(r.URL.Query().Get("fileName"))
	if fileName == "" {
		http.Error(w, `{"error":"fileName is required"}`, http.StatusBadRequest)
		return
	}

	file, err := os.Open(filepath.Join(h.archiveDir, fileName))
	if err != nil {
		http.Error(w, `{"error":"archive not found"}`, http.StatusNotFound)
		return
	}
	defer file.Close()

	var payload archivePayload
	if err := json.NewDecoder(file).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid archive JSON"}`, http.StatusBadRequest)
		return
	}

	principal := PrincipalFromContext(r.Context())
	tenantID := EffectiveTenant(principal)
	if !principal.IsGlobal && payload.TenantID != "" && payload.TenantID != tenantID {
		http.Error(w, `{"error":"archive tenant does not match caller tenant"}`, http.StatusForbidden)
		return
	}

	imported := 0
	for _, trace := range payload.Traces {
		if err := h.importTrace(principal, tenantID, trace); err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		imported++
	}
	writeJSON(w, map[string]any{"ok": true, "imported": imported, "fileName": fileName})
}

func (h *LifecycleHandler) importTrace(principal Principal, effectiveTenant string, dto TraceDetailDTO) error {
	trace, err := detailDTOToTrace(dto)
	if err != nil {
		return err
	}
	if !principal.IsGlobal {
		trace.TenantID = effectiveTenant
		for _, span := range trace.Spans {
			span.TenantID = effectiveTenant
		}
	} else if trace.TenantID == "" {
		trace.TenantID = effectiveTenant
		for _, span := range trace.Spans {
			span.TenantID = effectiveTenant
		}
	}

	trace.CriticalPath = h.analyzer.ComputeCriticalPath(trace)
	if trace.RootSpan != nil {
		trace.ParallelGroups = h.analyzer.DetectParallelGroups(trace.RootSpan)
	}
	trace.Gaps = h.analyzer.DetectGaps(trace)
	return h.store.Upsert(trace)
}

func detailDTOToTrace(dto TraceDetailDTO) (*model.Trace, error) {
	if dto.TraceID == "" {
		return nil, errors.New("traceId is required")
	}
	traceID, err := model.ParseTraceID(dto.TraceID)
	if err != nil {
		return nil, errors.New("invalid traceId")
	}

	spans := make([]*model.Span, 0, len(dto.Spans))
	spanMap := make(map[model.SpanID]*model.Span, len(dto.Spans))
	for _, spanDTO := range dto.Spans {
		spanID, err := model.ParseSpanID(spanDTO.SpanID)
		if err != nil {
			return nil, errors.New("invalid spanId")
		}
		span := &model.Span{
			TraceID:     traceID,
			SpanID:      spanID,
			TenantID:    coalesce(spanDTO.TenantID, dto.TenantID),
			Name:        spanDTO.Name,
			Kind:        model.SpanKind(spanDTO.Kind),
			ServiceName: spanDTO.ServiceName,
			StartTime:   time.Unix(0, int64(spanDTO.StartTimeUnixNano)),
			EndTime:     time.Unix(0, int64(spanDTO.StartTimeUnixNano)).Add(time.Duration(spanDTO.DurationMs * float64(time.Millisecond))),
			Attributes:  dtoToAttributes(spanDTO.Attributes),
			Status:      model.SpanStatus{Code: model.StatusCode(spanDTO.Status.Code), Message: spanDTO.Status.Message},
			HasError:    spanDTO.HasError,
			ReceivedAt:  time.Now(),
		}
		if spanDTO.ParentSpanID != "" {
			parentID, err := model.ParseSpanID(spanDTO.ParentSpanID)
			if err == nil {
				span.ParentSpanID = parentID
			}
		}
		for _, eventDTO := range spanDTO.Events {
			span.Events = append(span.Events, model.SpanEvent{
				Time:       time.Unix(0, int64(eventDTO.TimeUnixNano)),
				Name:       eventDTO.Name,
				Attributes: dtoToAttributes(eventDTO.Attributes),
			})
		}
		for _, linkDTO := range spanDTO.Links {
			linkTraceID, err1 := model.ParseTraceID(linkDTO.TraceID)
			linkSpanID, err2 := model.ParseSpanID(linkDTO.SpanID)
			if err1 == nil && err2 == nil {
				span.Links = append(span.Links, model.SpanLink{
					TraceID:    linkTraceID,
					SpanID:     linkSpanID,
					TraceState: linkDTO.TraceState,
					Attributes: dtoToAttributes(linkDTO.Attributes),
				})
			}
		}
		spans = append(spans, span)
		spanMap[spanID] = span
	}

	trace := &model.Trace{
		TraceID:     traceID,
		TenantID:    dto.TenantID,
		Spans:       spans,
		Services:    append([]string(nil), dto.Services...),
		SpanCount:   dto.SpanCount,
		ErrorCount:  dto.ErrorCount,
		Duration:    time.Duration(dto.DurationMs * float64(time.Millisecond)),
		ReceivedAt:  time.Now(),
		CompletedAt: time.Now(),
	}

	for _, span := range spans {
		if span.IsRoot() && (trace.RootSpan == nil || span.StartTime.Before(trace.RootSpan.StartTime)) {
			trace.RootSpan = span
		}
		if !span.ParentSpanID.IsZero() {
			if parent := spanMap[span.ParentSpanID]; parent != nil {
				parent.Children = append(parent.Children, span)
				span.Depth = parent.Depth + 1
			}
		}
	}

	return trace, nil
}

func sanitizeArchiveName(raw string) string {
	raw = strings.TrimSpace(filepath.Base(raw))
	if raw == "." || raw == "" {
		return ""
	}
	return raw
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
