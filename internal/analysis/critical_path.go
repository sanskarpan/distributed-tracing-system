package analysis

import (
	"sort"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// Analyzer provides trace analysis operations.
type Analyzer struct{}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer() *Analyzer { return &Analyzer{} }

// ComputeCriticalPath returns the sequence of causally-linked spans with maximum total duration.
// It traverses the tree selecting at each node the child subtree with the longest duration.
func (a *Analyzer) ComputeCriticalPath(trace *model.Trace) []*model.Span {
	if trace.RootSpan == nil {
		return nil
	}
	path, _ := longestPath(trace.RootSpan)
	return path
}

func longestPath(span *model.Span) (path []*model.Span, duration time.Duration) {
	if len(span.Children) == 0 {
		return []*model.Span{span}, span.Duration()
	}
	var bestChildPath []*model.Span
	var bestChildDur time.Duration
	for _, child := range span.Children {
		childPath, childDur := longestPath(child)
		if childDur > bestChildDur {
			bestChildDur = childDur
			bestChildPath = childPath
		}
	}
	return append([]*model.Span{span}, bestChildPath...), span.Duration() + bestChildDur
}

// DetectParallelGroups finds groups of sibling spans that overlap in time.
// It looks at the children of the given span and groups those with overlapping time ranges.
func (a *Analyzer) DetectParallelGroups(span *model.Span) []model.ParallelGroup {
	if len(span.Children) < 2 {
		return nil
	}

	// Sort children by start time
	children := make([]*model.Span, len(span.Children))
	copy(children, span.Children)
	sort.Slice(children, func(i, j int) bool {
		return children[i].StartTime.Before(children[j].StartTime)
	})

	var groups []model.ParallelGroup
	inGroup := make([]bool, len(children))

	for i := 0; i < len(children); i++ {
		if inGroup[i] {
			continue
		}
		group := []*model.Span{children[i]}
		groupEnd := children[i].EndTime
		groupStart := children[i].StartTime

		for j := i + 1; j < len(children); j++ {
			// Overlap: child j starts before child i ends
			if children[j].StartTime.Before(groupEnd) {
				group = append(group, children[j])
				inGroup[j] = true
				if children[j].EndTime.After(groupEnd) {
					groupEnd = children[j].EndTime
				}
			}
		}

		if len(group) > 1 {
			inGroup[i] = true
			groups = append(groups, model.ParallelGroup{
				Spans:     group,
				StartTime: groupStart,
				EndTime:   groupEnd,
			})
		}
	}

	return groups
}

// DetectGaps finds idle periods between sequential child spans of the root span.
// A gap occurs when one span ends before the next sibling starts.
func (a *Analyzer) DetectGaps(trace *model.Trace) []model.SpanGap {
	if trace.RootSpan == nil || len(trace.RootSpan.Children) < 2 {
		return nil
	}

	// Sort children by start time
	children := make([]*model.Span, len(trace.RootSpan.Children))
	copy(children, trace.RootSpan.Children)
	sort.Slice(children, func(i, j int) bool {
		return children[i].StartTime.Before(children[j].StartTime)
	})

	var gaps []model.SpanGap
	for i := 0; i < len(children)-1; i++ {
		cur := children[i]
		next := children[i+1]
		if next.StartTime.After(cur.EndTime) {
			gap := next.StartTime.Sub(cur.EndTime)
			if gap > 0 {
				gaps = append(gaps, model.SpanGap{
					Before:   cur,
					After:    next,
					Duration: gap,
				})
			}
		}
	}

	return gaps
}
