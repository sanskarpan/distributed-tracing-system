package model

import "time"

// Trace is the assembled set of spans sharing a TraceID.
type Trace struct {
	TraceID  TraceID
	Spans    []*Span // all spans, including orphans
	RootSpan *Span

	// Computed by assembler
	Services   []string      // unique service names sorted
	Duration   time.Duration // RootSpan.Duration() or max span end - min span start
	SpanCount  int
	ErrorCount int

	// Computed by analysis
	CriticalPath   []*Span
	ParallelGroups []ParallelGroup
	Gaps           []SpanGap

	ReceivedAt  time.Time
	CompletedAt time.Time
}

// ParallelGroup is a set of spans that execute concurrently.
type ParallelGroup struct {
	Spans     []*Span
	StartTime time.Time
	EndTime   time.Time
}

// SpanGap is an idle period between two spans.
type SpanGap struct {
	Before   *Span
	After    *Span
	Duration time.Duration
}
