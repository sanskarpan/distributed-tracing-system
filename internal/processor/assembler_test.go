package processor

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
)

func newSpanID(t *testing.T) model.SpanID {
	t.Helper()
	id, err := model.NewSpanID()
	require.NoError(t, err)
	return id
}

func newTraceID(t *testing.T) model.TraceID {
	t.Helper()
	id, err := model.NewTraceID()
	require.NoError(t, err)
	return id
}

func makeSpan(traceID model.TraceID, spanID model.SpanID, parentID model.SpanID, svc, name string, start, end time.Time) *model.Span {
	return &model.Span{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentID,
		ServiceName:  svc,
		Name:         name,
		StartTime:    start,
		EndTime:      end,
	}
}

// waitForTrace waits for the assembler to finalize and deliver a trace, up to maxWait.
func waitForTrace(ch <-chan *model.Trace, maxWait time.Duration) *model.Trace {
	select {
	case tr := <-ch:
		return tr
	case <-time.After(maxWait):
		return nil
	}
}

func TestAssembler_SingleSpan(t *testing.T) {
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	spanID := newSpanID(t)
	now := time.Now()
	span := makeSpan(traceID, spanID, model.SpanID{}, "svc-a", "op", now, now.Add(100*time.Millisecond))

	a.AddSpan(span)

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr, "expected trace to be finalized")
	assert.Equal(t, traceID, tr.TraceID)
	assert.Equal(t, 1, tr.SpanCount)
	assert.Equal(t, 1, len(tr.Spans))
	require.NotNil(t, tr.RootSpan)
	assert.Equal(t, spanID, tr.RootSpan.SpanID)
	assert.Equal(t, []string{"svc-a"}, tr.Services)
}

func TestAssembler_MultiSpanParentChild(t *testing.T) {
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	rootID := newSpanID(t)
	childID := newSpanID(t)
	grandchildID := newSpanID(t)

	now := time.Now()
	root := makeSpan(traceID, rootID, model.SpanID{}, "svc-a", "root-op", now, now.Add(300*time.Millisecond))
	child := makeSpan(traceID, childID, rootID, "svc-b", "child-op", now.Add(10*time.Millisecond), now.Add(200*time.Millisecond))
	grandchild := makeSpan(traceID, grandchildID, childID, "svc-c", "grandchild-op", now.Add(20*time.Millisecond), now.Add(150*time.Millisecond))

	a.AddSpan(root)
	a.AddSpan(child)
	a.AddSpan(grandchild)

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr)
	assert.Equal(t, 3, tr.SpanCount)
	require.NotNil(t, tr.RootSpan)
	assert.Equal(t, rootID, tr.RootSpan.SpanID)
	assert.Equal(t, 0, tr.RootSpan.Depth)

	require.Len(t, tr.RootSpan.Children, 1)
	assert.Equal(t, childID, tr.RootSpan.Children[0].SpanID)
	assert.Equal(t, 1, tr.RootSpan.Children[0].Depth)

	require.Len(t, tr.RootSpan.Children[0].Children, 1)
	assert.Equal(t, grandchildID, tr.RootSpan.Children[0].Children[0].SpanID)
	assert.Equal(t, 2, tr.RootSpan.Children[0].Children[0].Depth)

	assert.ElementsMatch(t, []string{"svc-a", "svc-b", "svc-c"}, tr.Services)
}

func TestAssembler_OutOfOrderSpans(t *testing.T) {
	// Child arrives before parent
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	rootID := newSpanID(t)
	childID := newSpanID(t)

	now := time.Now()
	child := makeSpan(traceID, childID, rootID, "svc-b", "child-op", now.Add(10*time.Millisecond), now.Add(100*time.Millisecond))
	root := makeSpan(traceID, rootID, model.SpanID{}, "svc-a", "root-op", now, now.Add(200*time.Millisecond))

	// Child arrives first
	a.AddSpan(child)
	a.AddSpan(root)

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr)
	assert.Equal(t, 2, tr.SpanCount)
	require.NotNil(t, tr.RootSpan)
	assert.Equal(t, rootID, tr.RootSpan.SpanID)
	require.Len(t, tr.RootSpan.Children, 1)
	assert.Equal(t, childID, tr.RootSpan.Children[0].SpanID)
}

func TestAssembler_OrphanSpan(t *testing.T) {
	// Span whose parent is not in the set — should still be stored in the trace
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	orphanID := newSpanID(t)
	missingParentID := newSpanID(t) // never added

	now := time.Now()
	orphan := makeSpan(traceID, orphanID, missingParentID, "svc-a", "orphan-op", now, now.Add(100*time.Millisecond))

	a.AddSpan(orphan)

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr)
	assert.Equal(t, 1, tr.SpanCount)
	assert.Equal(t, 1, len(tr.Spans))
	// Orphan span should be stored; RootSpan may be nil since it has a parent not in set
	found := false
	for _, s := range tr.Spans {
		if s.SpanID == orphanID {
			found = true
		}
	}
	assert.True(t, found, "orphan span should be stored in trace.Spans")
}

func TestAssembler_TimerReset(t *testing.T) {
	// Adding spans should reset the timer
	ch := make(chan *model.Trace, 1)
	timeout := 80 * time.Millisecond
	a := NewAssembler(timeout, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	now := time.Now()

	// Add first span
	s1, _ := model.NewSpanID()
	a.AddSpan(makeSpan(traceID, s1, model.SpanID{}, "svc", "op1", now, now.Add(10*time.Millisecond)))

	// Wait half the timeout, then add another span (resets timer)
	time.Sleep(40 * time.Millisecond)
	s2, _ := model.NewSpanID()
	a.AddSpan(makeSpan(traceID, s2, s1, "svc", "op2", now.Add(5*time.Millisecond), now.Add(15*time.Millisecond)))

	// Should NOT have fired yet
	select {
	case <-ch:
		t.Fatal("trace finalized too early")
	case <-time.After(50 * time.Millisecond):
		// good — timer was reset
	}

	// Now wait for it to actually fire
	tr := waitForTrace(ch, 300*time.Millisecond)
	require.NotNil(t, tr)
	assert.Equal(t, 2, tr.SpanCount)
}

func TestAssembler_HasError_PropagatesUp(t *testing.T) {
	// An error on a leaf span must propagate to the root span.
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	rootID := newSpanID(t)
	childID := newSpanID(t)
	now := time.Now()

	root := makeSpan(traceID, rootID, model.SpanID{}, "svc-a", "root-op", now, now.Add(200*time.Millisecond))
	child := makeSpan(traceID, childID, rootID, "svc-b", "child-op", now.Add(10*time.Millisecond), now.Add(100*time.Millisecond))
	child.Status = model.SpanStatus{Code: model.StatusError, Message: "something failed"}
	child.HasError = true

	a.AddSpan(root)
	a.AddSpan(child)

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr)
	assert.Greater(t, tr.ErrorCount, 0, "trace.ErrorCount must be > 0 when a span has StatusError")
	require.NotNil(t, tr.RootSpan)
	assert.True(t, tr.RootSpan.HasError, "root span.HasError must propagate up from child error")
}

func TestAssembler_PendingCount(t *testing.T) {
	var mu sync.Mutex
	completed := 0
	a := NewAssembler(50*time.Millisecond, func(tr *model.Trace) {
		mu.Lock()
		completed++
		mu.Unlock()
	})

	tid1 := newTraceID(t)
	tid2 := newTraceID(t)
	sid1, _ := model.NewSpanID()
	sid2, _ := model.NewSpanID()
	now := time.Now()

	a.AddSpan(makeSpan(tid1, sid1, model.SpanID{}, "svc", "op", now, now.Add(10*time.Millisecond)))
	a.AddSpan(makeSpan(tid2, sid2, model.SpanID{}, "svc", "op", now, now.Add(10*time.Millisecond)))

	assert.Equal(t, 2, a.PendingCount())

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, a.PendingCount())
	mu.Lock()
	assert.Equal(t, 2, completed)
	mu.Unlock()
}

func TestAssembler_StopFlushesPendingTraces(t *testing.T) {
	ch := make(chan *model.Trace, 1)
	a := NewAssembler(10*time.Second, func(tr *model.Trace) {
		ch <- tr
	})

	traceID := newTraceID(t)
	spanID := newSpanID(t)
	now := time.Now()

	a.AddSpan(makeSpan(traceID, spanID, model.SpanID{}, "svc-a", "op", now, now.Add(100*time.Millisecond)))
	assert.Equal(t, 1, a.PendingCount())

	a.Stop()

	tr := waitForTrace(ch, 500*time.Millisecond)
	require.NotNil(t, tr, "expected Stop to flush pending traces")
	assert.Equal(t, traceID, tr.TraceID)
	assert.Equal(t, 0, a.PendingCount())
}
