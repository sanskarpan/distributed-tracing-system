package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/yourname/tracing/internal/metrics"
)

type Alert struct {
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Severity    string            `json:"severity"`
	TenantID    string            `json:"tenantId,omitempty"`
	Service     string            `json:"service,omitempty"`
	Operation   string            `json:"operation,omitempty"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels,omitempty"`
	ActiveSince time.Time         `json:"activeSince"`
	LastSentAt  time.Time         `json:"lastSentAt,omitempty"`
}

type AlertManager struct {
	metricsStore *metrics.MetricsStore
	probes       *ProbeState
	webhookURL   string
	cooldown     time.Duration
	evalInterval time.Duration
	sloTarget    float64
	anomalyZ     float64
	httpClient   *http.Client

	mu     sync.RWMutex
	active map[string]Alert
}

func NewAlertManager(metricsStore *metrics.MetricsStore, probes *ProbeState) *AlertManager {
	return &AlertManager{
		metricsStore: metricsStore,
		probes:       probes,
		webhookURL:   os.Getenv("ALERT_WEBHOOK_URL"),
		cooldown:     envDurationWithDefault("ALERT_COOLDOWN", 2*time.Minute),
		evalInterval: envDurationWithDefault("ALERT_EVAL_INTERVAL", 15*time.Second),
		sloTarget:    envFloatWithDefault("ALERT_SLO_TARGET", 0.01),
		anomalyZ:     envFloatWithDefault("ALERT_ANOMALY_Z", 2),
		httpClient: &http.Client{
			Timeout: envDurationWithDefault("ALERT_WEBHOOK_TIMEOUT", 5*time.Second),
		},
		active: make(map[string]Alert),
	}
}

func (m *AlertManager) SetProbes(probes *ProbeState) {
	if m == nil {
		return
	}
	m.probes = probes
}

func (m *AlertManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(m.evalInterval)
		defer ticker.Stop()
		m.evaluateAndNotify(time.Now())
		for {
			select {
			case <-ticker.C:
				m.evaluateAndNotify(time.Now())
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (m *AlertManager) Snapshot() []Alert {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	alerts := make([]Alert, 0, len(m.active))
	for _, alert := range m.active {
		alerts = append(alerts, alert)
	}
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity > alerts[j].Severity
		}
		if alerts[i].TenantID != alerts[j].TenantID {
			return alerts[i].TenantID < alerts[j].TenantID
		}
		return alerts[i].Key < alerts[j].Key
	})
	return alerts
}

func (m *AlertManager) HandleGetAlerts(w http.ResponseWriter, r *http.Request) {
	principal := PrincipalFromContext(r.Context())
	tenantID := EffectiveTenant(principal)
	alerts := m.Snapshot()
	if tenantID != "" && !principal.IsGlobal {
		filtered := alerts[:0]
		for _, alert := range alerts {
			if alert.TenantID == "" || alert.TenantID == tenantID {
				filtered = append(filtered, alert)
			}
		}
		alerts = filtered
	}
	writeJSON(w, map[string]any{"alerts": alerts})
}

func (m *AlertManager) evaluateAndNotify(now time.Time) {
	if m == nil {
		return
	}

	next := make(map[string]Alert)
	for _, alert := range m.evaluate(now) {
		next[alert.Key] = alert
	}

	var toSend []Alert

	m.mu.Lock()
	for key, alert := range next {
		if previous, ok := m.active[key]; ok {
			alert.ActiveSince = previous.ActiveSince
			alert.LastSentAt = previous.LastSentAt
			if m.webhookURL != "" && (previous.LastSentAt.IsZero() || now.Sub(previous.LastSentAt) >= m.cooldown) {
				alert.LastSentAt = now
				toSend = append(toSend, alert)
			}
		} else {
			alert.ActiveSince = now
			if m.webhookURL != "" {
				alert.LastSentAt = now
				toSend = append(toSend, alert)
			}
		}
		next[key] = alert
	}
	m.active = next
	m.mu.Unlock()

	if len(toSend) > 0 {
		m.sendWebhook(toSend, now)
	}
}

func (m *AlertManager) evaluate(now time.Time) []Alert {
	var alerts []Alert
	if m.probes == nil {
		return alerts
	}
	snapshot := m.probes.Snapshot()
	switch snapshot.Status {
	case "overloaded":
		alerts = append(alerts, Alert{
			Key:      "collector:queue-overload",
			Type:     "queue_overload",
			Severity: "critical",
			Message:  "collector readiness dropped because worker queue usage crossed the configured threshold",
			Labels: map[string]string{
				"queueDepth":    strconv.Itoa(snapshot.QueueDepth),
				"queueCapacity": strconv.Itoa(snapshot.QueueCapacity),
			},
		})
	case "draining":
		alerts = append(alerts, Alert{
			Key:      "collector:draining",
			Type:     "collector_draining",
			Severity: "warning",
			Message:  "collector is draining and should stop receiving new traffic",
		})
	}

	for _, result := range m.metricsStore.ComputeSLOs(m.sloTarget, "") {
		if !result.Breached {
			continue
		}
		key := "slo:" + result.Service
		alerts = append(alerts, Alert{
			Key:      key,
			Type:     "slo_breach",
			Severity: "critical",
			Service:  result.Service,
			Message:  "service error budget is currently breached",
		})
	}

	for _, result := range m.metricsStore.DetectAnomalies(m.anomalyZ, "") {
		if !result.IsOutlier {
			continue
		}
		key := "anomaly:" + result.Service + ":" + result.Operation
		alerts = append(alerts, Alert{
			Key:       key,
			Type:      "latency_anomaly",
			Severity:  "warning",
			Service:   result.Service,
			Operation: result.Operation,
			Message:   "operation latency is an outlier compared with the current population",
		})
	}

	return alerts
}

func (m *AlertManager) sendWebhook(alerts []Alert, now time.Time) {
	payload, err := json.Marshal(map[string]any{
		"triggeredAt": now.UTC(),
		"alerts":      alerts,
	})
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, m.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func envDurationWithDefault(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envFloatWithDefault(key string, fallback float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
