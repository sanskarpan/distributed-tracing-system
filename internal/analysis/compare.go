package analysis

import (
	"fmt"
	"sort"
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

// matchKey identifies structurally equivalent spans across traces.
// It includes the full ancestry path so repeated operations under different
// parents are not collapsed into the same comparison bucket.
type matchKey string

// CompareTraces compares two traces using ancestry-aware span grouping.
// Spans within the same structural group are matched by their start time,
// which makes retries and fan-out comparisons stable even if slice order differs.
func (a *Analyzer) CompareTraces(base, compare *model.Trace) *TraceComparison {
	result := &TraceComparison{}

	// Compute top-level deltas
	result.DurationDeltaMs = durationMs(compare.Duration) - durationMs(base.Duration)
	result.SpanCountDelta = compare.SpanCount - base.SpanCount
	result.ErrorDelta = compare.ErrorCount - base.ErrorCount

	baseGroups := groupSpansForComparison(base.Spans)
	cmpGroups := groupSpansForComparison(compare.Spans)

	allKeys := make(map[matchKey]struct{})
	for k := range baseGroups {
		allKeys[k] = struct{}{}
	}
	for k := range cmpGroups {
		allKeys[k] = struct{}{}
	}

	keys := make([]matchKey, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for _, k := range keys {
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

func groupSpansForComparison(spans []*model.Span) map[matchKey][]*model.Span {
	parentByID := make(map[model.SpanID]*model.Span, len(spans))
	for _, span := range spans {
		parentByID[span.SpanID] = span
	}

	groups := make(map[matchKey][]*model.Span)
	cache := make(map[model.SpanID]matchKey, len(spans))
	visiting := make(map[model.SpanID]bool, len(spans))

	for _, span := range spans {
		key := spanMatchKey(span, parentByID, cache, visiting)
		groups[key] = append(groups[key], span)
	}

	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			return spanOrderLess(group[i], group[j])
		})
	}

	return groups
}

func spanMatchKey(span *model.Span, parentByID map[model.SpanID]*model.Span, cache map[model.SpanID]matchKey, visiting map[model.SpanID]bool) matchKey {
	if key, ok := cache[span.SpanID]; ok {
		return key
	}
	if visiting[span.SpanID] {
		return matchKey(spanSegment(span))
	}

	visiting[span.SpanID] = true
	segment := matchKey(spanSegment(span))

	var key matchKey
	switch {
	case span.ParentSpanID.IsZero():
		key = segment
	case parentByID[span.ParentSpanID] == nil:
		key = matchKey(fmt.Sprintf("missing:%s>%s", span.ParentSpanID.String(), segment))
	default:
		key = spanMatchKey(parentByID[span.ParentSpanID], parentByID, cache, visiting) + ">" + segment
	}

	delete(visiting, span.SpanID)
	cache[span.SpanID] = key
	return key
}

func spanSegment(span *model.Span) string {
	return fmt.Sprintf("%s/%s", span.ServiceName, span.Name)
}

func spanOrderLess(left, right *model.Span) bool {
	if !left.StartTime.Equal(right.StartTime) {
		return left.StartTime.Before(right.StartTime)
	}
	if !left.EndTime.Equal(right.EndTime) {
		return left.EndTime.Before(right.EndTime)
	}
	if left.ServiceName != right.ServiceName {
		return left.ServiceName < right.ServiceName
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.SpanID.String() < right.SpanID.String()
}
