package processor

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// Assembler groups spans by TraceID and assembles them into complete Traces.
// After a quiet period (no new spans for timeout duration), it finalizes the trace.
type Assembler struct {
	mu         sync.Mutex
	pending    map[model.TraceID]*pendingTrace
	timeout    time.Duration
	onComplete func(*model.Trace)
}

type pendingTrace struct {
	spans  []*model.Span
	timer  *time.Timer
	lastAt time.Time
}

// NewAssembler creates a new Assembler with the given quiet-period timeout and completion callback.
func NewAssembler(timeout time.Duration, onComplete func(*model.Trace)) *Assembler {
	return &Assembler{
		pending:    make(map[model.TraceID]*pendingTrace),
		timeout:    timeout,
		onComplete: onComplete,
	}
}

// AddSpan adds a span to the pending set for its TraceID, resetting the quiet-period timer.
func (a *Assembler) AddSpan(span *model.Span) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pt, ok := a.pending[span.TraceID]
	if !ok {
		pt = &pendingTrace{
			lastAt: time.Now(),
		}
		pt.timer = time.AfterFunc(a.timeout, func() {
			a.finalize(span.TraceID)
		})
		a.pending[span.TraceID] = pt
	} else {
		pt.timer.Reset(a.timeout)
		pt.lastAt = time.Now()
	}
	pt.spans = append(pt.spans, span)
}

// finalize assembles the pending spans into a trace and calls onComplete.
func (a *Assembler) finalize(traceID model.TraceID) {
	a.mu.Lock()
	pt, ok := a.pending[traceID]
	if !ok {
		a.mu.Unlock()
		return
	}
	spans := pt.spans
	delete(a.pending, traceID)
	a.mu.Unlock()

	trace := a.buildTree(spans)
	if a.onComplete != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("assembler: onComplete panic for trace %s: %v", traceID, r)
				}
			}()
			a.onComplete(trace)
		}()
	}
}

// buildTree constructs a Trace from a flat list of spans, linking children to parents
// and computing aggregate fields.
func (a *Assembler) buildTree(spans []*model.Span) *model.Trace {
	trace := &model.Trace{
		Spans:       spans,
		SpanCount:   len(spans),
		ReceivedAt:  time.Now(),
		CompletedAt: time.Now(),
	}

	if len(spans) == 0 {
		return trace
	}

	trace.TraceID = spans[0].TraceID

	// Build spanMap by SpanID
	spanMap := make(map[model.SpanID]*model.Span, len(spans))
	for _, s := range spans {
		spanMap[s.SpanID] = s
		s.Children = nil // reset children
		s.Depth = 0
	}

	// Link children to parents
	var roots []*model.Span
	var orphans []*model.Span
	for _, s := range spans {
		if s.IsRoot() {
			roots = append(roots, s)
		} else {
			parent, found := spanMap[s.ParentSpanID]
			if found {
				parent.Children = append(parent.Children, s)
			} else {
				orphans = append(orphans, s)
			}
		}
	}

	// Pick the single root (if multiple root-like spans, use earliest start time)
	if len(roots) == 1 {
		trace.RootSpan = roots[0]
	} else if len(roots) > 1 {
		sort.Slice(roots, func(i, j int) bool {
			return roots[i].StartTime.Before(roots[j].StartTime)
		})
		trace.RootSpan = roots[0]
		// Make the other "root" spans children of the first root
		// (they are orphan roots — no parent in set)
		for _, r := range roots[1:] {
			orphans = append(orphans, r)
		}
	}

	// Iterative BFS: assign depths and sort children.
	// Avoids stack overflow for pathologically deep span chains.
	type bfsFrame struct {
		span  *model.Span
		depth int
	}
	bfsQueue := make([]bfsFrame, 0, len(spans))
	if trace.RootSpan != nil {
		bfsQueue = append(bfsQueue, bfsFrame{trace.RootSpan, 0})
	}
	for _, o := range orphans {
		bfsQueue = append(bfsQueue, bfsFrame{o, 0})
	}
	bfsOrder := make([]*model.Span, 0, len(spans))
	for len(bfsQueue) > 0 {
		f := bfsQueue[0]
		bfsQueue = bfsQueue[1:]
		f.span.Depth = f.depth
		sort.Slice(f.span.Children, func(i, j int) bool {
			return f.span.Children[i].StartTime.Before(f.span.Children[j].StartTime)
		})
		bfsOrder = append(bfsOrder, f.span)
		for _, child := range f.span.Children {
			bfsQueue = append(bfsQueue, bfsFrame{child, f.depth + 1})
		}
	}

	if trace.RootSpan != nil {
		trace.Duration = trace.RootSpan.Duration()
	}

	// Propagate HasError upward in reverse BFS order (leaves before parents).
	for i := len(bfsOrder) - 1; i >= 0; i-- {
		s := bfsOrder[i]
		for _, child := range s.Children {
			if child.HasError {
				s.HasError = true
			}
		}
	}

	// If no root found, compute duration from min/max times
	if trace.RootSpan == nil && len(spans) > 0 {
		minStart := spans[0].StartTime
		maxEnd := spans[0].EndTime
		for _, s := range spans[1:] {
			if s.StartTime.Before(minStart) {
				minStart = s.StartTime
			}
			if s.EndTime.After(maxEnd) {
				maxEnd = s.EndTime
			}
		}
		trace.Duration = maxEnd.Sub(minStart)
	}

	// Compute Services (unique, sorted)
	serviceSet := make(map[string]struct{})
	for _, s := range spans {
		if s.ServiceName != "" {
			serviceSet[s.ServiceName] = struct{}{}
		}
	}
	for svc := range serviceSet {
		trace.Services = append(trace.Services, svc)
	}
	sort.Strings(trace.Services)

	// Compute ErrorCount
	for _, s := range spans {
		if s.Status.Code == model.StatusError {
			trace.ErrorCount++
		}
	}

	return trace
}

// PendingCount returns the number of trace IDs currently pending assembly.
func (a *Assembler) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

// Stop flushes all pending traces immediately and stops their quiet-period timers.
func (a *Assembler) Stop() {
	a.mu.Lock()
	ids := make([]model.TraceID, 0, len(a.pending))
	for traceID, pt := range a.pending {
		if pt.timer != nil {
			pt.timer.Stop()
		}
		ids = append(ids, traceID)
	}
	a.mu.Unlock()

	for _, traceID := range ids {
		a.finalize(traceID)
	}
}
