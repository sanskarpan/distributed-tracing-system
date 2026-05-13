package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyzReturnsReadyWithQueueMetadata(t *testing.T) {
	probes := NewProbeState(func() int { return 12 }, 120)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	probes.HandleReadyz(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ready", body["status"])
	assert.Equal(t, float64(12), body["queueDepth"])
	assert.Equal(t, float64(120), body["queueCapacity"])
	assert.Equal(t, float64(10), body["queueUsagePct"])
}

func TestReadyzReturnsServiceUnavailableWhenQueueUsageExceedsThreshold(t *testing.T) {
	t.Setenv("READINESS_MAX_QUEUE_USAGE_PCT", "50")
	probes := NewProbeState(func() int { return 60 }, 100)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	probes.HandleReadyz(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "overloaded", body["status"])
	assert.Equal(t, float64(60), body["queueDepth"])
	assert.Equal(t, float64(50), body["queueThreshold"])
}
