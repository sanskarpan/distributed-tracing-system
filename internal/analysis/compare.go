package analysis

import (
	"time"

	"github.com/yourname/tracing/internal/model"
)

// SpanDiff holds the duration difference for a matched pair of spans.
type SpanDiff struct {
	BaseSpanID      model.SpanID
	CompareSpanID   model.SpanID
	DurationDeltaMs float64
}

// TraceComparison is the result of comparing two traces.
type TraceComparison struct {
	DurationDeltaMs float64
	SpanCountDelta  int
	ErrorDelta      int
	Matched         []SpanDiff
	OnlyInBase      []model.SpanID
	OnlyInCompare   []model.SpanID
}

// matchKey is the key used to correlate spans between traces.
type matchKey struct {
	ServiceName string
	Operation   string
}

// CompareTraces compares two traces, matching spans by (serviceName, operationName).
// When multiple spans share the same key in a trace, they are matched positionally.
func (a *Analyzer) CompareTraces(base, compare *model.Trace) *TraceComparison {
	result := &TraceComparison{}

	// Compute top-level deltas
	result.DurationDeltaMs = durationMs(compare.Duration) - durationMs(base.Duration)
	result.SpanCountDelta = compare.SpanCount - base.SpanCount
	result.ErrorDelta = compare.ErrorCount - base.ErrorCount

	// Group base spans by key
	baseGroups := make(map[matchKey][]*model.Span)
	for _, s := range base.Spans {
		k := matchKey{ServiceName: s.ServiceName, Operation: s.Name}
		baseGroups[k] = append(baseGroups[k], s)
	}

	// Group compare spans by key
	cmpGroups := make(map[matchKey][]*model.Span)
	for _, s := range compare.Spans {
		k := matchKey{ServiceName: s.ServiceName, Operation: s.Name}
		cmpGroups[k] = append(cmpGroups[k], s)
	}

	// Match spans positionally within each key
	allKeys := make(map[matchKey]struct{})
	for k := range baseGroups {
		allKeys[k] = struct{}{}
	}
	for k := range cmpGroups {
		allKeys[k] = struct{}{}
	}

	for k := range allKeys {
		bSpans := baseGroups[k]
		cSpans := cmpGroups[k]

		minLen := len(bSpans)
		if len(cSpans) < minLen {
			minLen = len(cSpans)
		}

		// Matched pairs
		for i := 0; i < minLen; i++ {
			delta := durationMs(cSpans[i].Duration()) - durationMs(bSpans[i].Duration())
			result.Matched = append(result.Matched, SpanDiff{
				BaseSpanID:      bSpans[i].SpanID,
				CompareSpanID:   cSpans[i].SpanID,
				DurationDeltaMs: delta,
			})
		}

		// Only in base
		for i := minLen; i < len(bSpans); i++ {
			result.OnlyInBase = append(result.OnlyInBase, bSpans[i].SpanID)
		}

		// Only in compare
		for i := minLen; i < len(cSpans); i++ {
			result.OnlyInCompare = append(result.OnlyInCompare, cSpans[i].SpanID)
		}
	}

	return result
}

func durationMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
