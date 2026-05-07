package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/tracing/internal/model"
)

func TestTraceID_Uniqueness(t *testing.T) {
	seen := make(map[model.TraceID]struct{}, 10000)
	for i := 0; i < 10000; i++ {
		id, err := model.NewTraceID()
		require.NoError(t, err)
		_, exists := seen[id]
		assert.False(t, exists, "duplicate TraceID at iteration %d", i)
		seen[id] = struct{}{}
	}
}

func TestTraceID_ParseRoundtrip(t *testing.T) {
	id, err := model.NewTraceID()
	require.NoError(t, err)
	s := id.String()
	assert.Len(t, s, 32)

	parsed, err := model.ParseTraceID(s)
	require.NoError(t, err)
	assert.Equal(t, id, parsed)
}

func TestTraceID_ParseRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"4bf92f3577b34da6a3ce929d0e0e473",   // 31 chars
		"4bf92f3577b34da6a3ce929d0e0e47366",  // 33 chars
		"4bf92f3577b34da6a3ce929d0e0e473g",   // non-hex
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",   // non-hex 32
	}
	for _, s := range cases {
		_, err := model.ParseTraceID(s)
		assert.Error(t, err, "expected error for %q", s)
	}
}

func TestSpanID_ParseRoundtrip(t *testing.T) {
	id, err := model.NewSpanID()
	require.NoError(t, err)
	s := id.String()
	assert.Len(t, s, 16)

	parsed, err := model.ParseSpanID(s)
	require.NoError(t, err)
	assert.Equal(t, id, parsed)
}

func TestSpanID_ParseRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"00f067aa0ba902",   // 14 chars
		"00f067aa0ba902b7f", // 17 chars
		"00f067aa0ba902gz",  // non-hex
	}
	for _, s := range cases {
		_, err := model.ParseSpanID(s)
		assert.Error(t, err, "expected error for %q", s)
	}
}

func TestTraceID_IsZero(t *testing.T) {
	assert.True(t, model.ZeroTraceID.IsZero())
	id, err := model.NewTraceID()
	require.NoError(t, err)
	assert.False(t, id.IsZero())
}

func TestSpanID_IsZero(t *testing.T) {
	assert.True(t, model.ZeroSpanID.IsZero())
	id, err := model.NewSpanID()
	require.NoError(t, err)
	assert.False(t, id.IsZero())
}

func TestSpan_IsRoot(t *testing.T) {
	span := &model.Span{}
	assert.True(t, span.IsRoot(), "zero ParentSpanID should be root")

	parentID, _ := model.NewSpanID()
	span.ParentSpanID = parentID
	assert.False(t, span.IsRoot())
}

func TestSpan_JSONMarshalUnmarshal(t *testing.T) {
	traceID, err := model.NewTraceID()
	require.NoError(t, err)
	spanID, err := model.NewSpanID()
	require.NoError(t, err)
	parentID, err := model.NewSpanID()
	require.NoError(t, err)

	original := &model.Span{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentID,
		Name:         "test span",
	}

	b, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded struct {
		TraceID      model.TraceID
		SpanID       model.SpanID
		ParentSpanID model.SpanID
		Name         string
	}
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, traceID, decoded.TraceID)
	assert.Equal(t, spanID, decoded.SpanID)
	assert.Equal(t, parentID, decoded.ParentSpanID)
	assert.Equal(t, "test span", decoded.Name)
}
