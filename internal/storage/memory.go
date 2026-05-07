package storage

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// MemoryStore is a thread-safe in-memory TraceStore with multi-field indexes and LRU eviction.
type MemoryStore struct {
	mu          sync.RWMutex
	traces      map[model.TraceID]*model.Trace
	byService   map[string]map[model.TraceID]struct{}
	byOperation map[string]map[model.TraceID]struct{} // "service:operation" → set
	byError     map[model.TraceID]struct{}
	timeline    []timelineEntry // sorted by ReceivedAt ascending for eviction
	maxSize     int
}

type timelineEntry struct {
	id         model.TraceID
	receivedAt time.Time
}

// NewMemoryStore creates a new MemoryStore with the given maximum capacity.
func NewMemoryStore(maxSize int) *MemoryStore {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MemoryStore{
		traces:      make(map[model.TraceID]*model.Trace),
		byService:   make(map[string]map[model.TraceID]struct{}),
		byOperation: make(map[string]map[model.TraceID]struct{}),
		byError:     make(map[model.TraceID]struct{}),
		maxSize:     maxSize,
	}
}

// Upsert adds or updates a trace, evicting the oldest if over maxSize.
func (s *MemoryStore) Upsert(trace *model.Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := trace.TraceID

	// If updating existing, remove old index entries first
	if old, exists := s.traces[id]; exists {
		s.removeFromIndexes(old)
		// Remove from timeline
		for i, e := range s.timeline {
			if e.id == id {
				s.timeline = append(s.timeline[:i], s.timeline[i+1:]...)
				break
			}
		}
	}

	// Evict oldest if at capacity
	for len(s.traces) >= s.maxSize {
		s.evict()
	}

	s.traces[id] = trace

	// Index by service
	for _, svc := range trace.Services {
		if s.byService[svc] == nil {
			s.byService[svc] = make(map[model.TraceID]struct{})
		}
		s.byService[svc][id] = struct{}{}
	}

	// Index by operation: "service:operation" for root span
	if trace.RootSpan != nil {
		key := fmt.Sprintf("%s:%s", trace.RootSpan.ServiceName, trace.RootSpan.Name)
		if s.byOperation[key] == nil {
			s.byOperation[key] = make(map[model.TraceID]struct{})
		}
		s.byOperation[key][id] = struct{}{}

		// Also index all spans' operations
		for _, span := range trace.Spans {
			opKey := fmt.Sprintf("%s:%s", span.ServiceName, span.Name)
			if s.byOperation[opKey] == nil {
				s.byOperation[opKey] = make(map[model.TraceID]struct{})
			}
			s.byOperation[opKey][id] = struct{}{}
		}
	} else {
		for _, span := range trace.Spans {
			opKey := fmt.Sprintf("%s:%s", span.ServiceName, span.Name)
			if s.byOperation[opKey] == nil {
				s.byOperation[opKey] = make(map[model.TraceID]struct{})
			}
			s.byOperation[opKey][id] = struct{}{}
		}
	}

	// Index by error
	if trace.ErrorCount > 0 {
		s.byError[id] = struct{}{}
	}

	// Insert into timeline (sorted by ReceivedAt)
	entry := timelineEntry{id: id, receivedAt: trace.ReceivedAt}
	pos := sort.Search(len(s.timeline), func(i int) bool {
		return s.timeline[i].receivedAt.After(trace.ReceivedAt)
	})
	s.timeline = append(s.timeline, timelineEntry{})
	copy(s.timeline[pos+1:], s.timeline[pos:])
	s.timeline[pos] = entry

	return nil
}

// removeFromIndexes removes a trace from all secondary indexes (not the main map).
func (s *MemoryStore) removeFromIndexes(trace *model.Trace) {
	id := trace.TraceID

	for _, svc := range trace.Services {
		if m, ok := s.byService[svc]; ok {
			delete(m, id)
			if len(m) == 0 {
				delete(s.byService, svc)
			}
		}
	}

	// Remove from operation indexes
	seen := make(map[string]struct{})
	for _, span := range trace.Spans {
		opKey := fmt.Sprintf("%s:%s", span.ServiceName, span.Name)
		if _, already := seen[opKey]; already {
			continue
		}
		seen[opKey] = struct{}{}
		if m, ok := s.byOperation[opKey]; ok {
			delete(m, id)
			if len(m) == 0 {
				delete(s.byOperation, opKey)
			}
		}
	}

	delete(s.byError, id)
}

// evict removes the oldest trace (by ReceivedAt) from all indexes.
func (s *MemoryStore) evict() {
	if len(s.timeline) == 0 {
		return
	}
	oldest := s.timeline[0]
	s.timeline = s.timeline[1:]

	if trace, ok := s.traces[oldest.id]; ok {
		s.removeFromIndexes(trace)
		delete(s.traces, oldest.id)
	}
}

// Get retrieves a trace by its TraceID.
func (s *MemoryStore) Get(id model.TraceID) (*model.Trace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tr, ok := s.traces[id]
	return tr, ok
}

// Query returns traces matching the given query parameters.
func (s *MemoryStore) Query(q *TraceQuery) (*TraceQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Start with candidate set
	var candidates map[model.TraceID]struct{}

	// If TraceID is specified, short-circuit
	if q.TraceID != nil {
		if tr, ok := s.traces[*q.TraceID]; ok {
			summary := s.toSummary(tr)
			return &TraceQueryResult{
				Traces:  []*TraceSummary{summary},
				Total:   1,
				HasMore: false,
			}, nil
		}
		return &TraceQueryResult{Traces: []*TraceSummary{}, Total: 0, HasMore: false}, nil
	}

	if q.ServiceName != "" {
		if m, ok := s.byService[q.ServiceName]; ok {
			candidates = copySet(m)
		} else {
			return &TraceQueryResult{Traces: []*TraceSummary{}, Total: 0, HasMore: false}, nil
		}
	} else {
		// All traces
		candidates = make(map[model.TraceID]struct{}, len(s.traces))
		for id := range s.traces {
			candidates[id] = struct{}{}
		}
	}

	// Intersect with operation filter
	if q.OperationName != "" {
		var opSet map[model.TraceID]struct{}
		if q.ServiceName != "" {
			key := fmt.Sprintf("%s:%s", q.ServiceName, q.OperationName)
			opSet = s.byOperation[key]
		} else {
			// Collect all service:op entries matching the operation name
			opSet = make(map[model.TraceID]struct{})
			for key, m := range s.byOperation {
				// key is "service:operation"; check if suffix matches
				opName := operationFromKey(key)
				if opName == q.OperationName {
					for id := range m {
						opSet[id] = struct{}{}
					}
				}
			}
		}
		candidates = intersectSets(candidates, opSet)
	}

	// Intersect with error filter
	if q.HasError != nil {
		if *q.HasError {
			candidates = intersectSets(candidates, s.byError)
		} else {
			for id := range s.byError {
				delete(candidates, id)
			}
		}
	}

	// Load traces and apply remaining filters
	var filtered []*model.Trace
	for id := range candidates {
		tr, ok := s.traces[id]
		if !ok {
			continue
		}

		if q.MinDuration != nil && tr.Duration < *q.MinDuration {
			continue
		}
		if q.MaxDuration != nil && tr.Duration > *q.MaxDuration {
			continue
		}
		if q.StartTime != nil && tr.ReceivedAt.Before(*q.StartTime) {
			continue
		}
		if q.EndTime != nil && tr.ReceivedAt.After(*q.EndTime) {
			continue
		}

		filtered = append(filtered, tr)
	}

	// Sort
	switch q.SortBy {
	case "duration":
		sort.Slice(filtered, func(i, j int) bool {
			if q.SortDesc {
				return filtered[i].Duration > filtered[j].Duration
			}
			return filtered[i].Duration < filtered[j].Duration
		})
	case "spanCount":
		sort.Slice(filtered, func(i, j int) bool {
			if q.SortDesc {
				return filtered[i].SpanCount > filtered[j].SpanCount
			}
			return filtered[i].SpanCount < filtered[j].SpanCount
		})
	default: // "receivedAt" or default
		sort.Slice(filtered, func(i, j int) bool {
			if q.SortDesc {
				return filtered[i].ReceivedAt.After(filtered[j].ReceivedAt)
			}
			return filtered[i].ReceivedAt.Before(filtered[j].ReceivedAt)
		})
	}

	total := len(filtered)

	// Apply offset and limit
	if q.Offset > 0 {
		if q.Offset >= len(filtered) {
			filtered = nil
		} else {
			filtered = filtered[q.Offset:]
		}
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}

	hasMore := false
	if len(filtered) > limit {
		filtered = filtered[:limit]
		hasMore = true
	}

	summaries := make([]*TraceSummary, len(filtered))
	for i, tr := range filtered {
		summaries[i] = s.toSummary(tr)
	}

	return &TraceQueryResult{
		Traces:  summaries,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

// Services returns a sorted list of all known service names.
func (s *MemoryStore) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	svcs := make([]string, 0, len(s.byService))
	for svc := range s.byService {
		svcs = append(svcs, svc)
	}
	sort.Strings(svcs)
	return svcs
}

// Operations returns a sorted list of all known operation names for a given service.
func (s *MemoryStore) Operations(service string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := service + ":"
	seen := make(map[string]struct{})
	for key, m := range s.byOperation {
		if len(m) == 0 {
			continue
		}
		if service == "" {
			op := operationFromKey(key)
			seen[op] = struct{}{}
		} else if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			op := key[len(prefix):]
			seen[op] = struct{}{}
		}
	}
	ops := make([]string, 0, len(seen))
	for op := range seen {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// Stats returns operational statistics for the store.
func (s *MemoryStore) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StoreStats{
		TraceCount: len(s.traces),
		MaxSize:    s.maxSize,
	}
}

// toSummary builds a TraceSummary from a Trace (must be called under read lock).
func (s *MemoryStore) toSummary(tr *model.Trace) *TraceSummary {
	sum := &TraceSummary{
		TraceID:    tr.TraceID,
		Duration:   tr.Duration,
		SpanCount:  tr.SpanCount,
		Services:   tr.Services,
		HasError:   tr.ErrorCount > 0,
		ReceivedAt: tr.ReceivedAt,
	}
	if tr.RootSpan != nil {
		sum.RootService = tr.RootSpan.ServiceName
		sum.RootOp = tr.RootSpan.Name
	}
	return sum
}

// copySet returns a shallow copy of a set.
func copySet(m map[model.TraceID]struct{}) map[model.TraceID]struct{} {
	out := make(map[model.TraceID]struct{}, len(m))
	for k := range m {
		out[k] = struct{}{}
	}
	return out
}

// intersectSets returns a new set containing only keys present in both a and b.
func intersectSets(a, b map[model.TraceID]struct{}) map[model.TraceID]struct{} {
	out := make(map[model.TraceID]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

// operationFromKey extracts the operation name from a "service:operation" index key.
func operationFromKey(key string) string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[i+1:]
		}
	}
	return key
}
