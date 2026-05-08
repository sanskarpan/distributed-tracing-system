package api

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yourname/tracing/internal/metrics"
)

// prometheusCollector bridges our MetricsStore and Pipeline into Prometheus.
type prometheusCollector struct {
	metricsStore *metrics.MetricsStore
	pipeline     *Pipeline

	requestRate  *prometheus.Desc
	errorRate    *prometheus.Desc
	latencyP50   *prometheus.Desc
	latencyP95   *prometheus.Desc
	latencyP99   *prometheus.Desc
	sampledTotal *prometheus.Desc
	droppedTotal *prometheus.Desc
}

func newPrometheusCollector(ms *metrics.MetricsStore, p *Pipeline) *prometheusCollector {
	labels := []string{"service", "operation"}
	return &prometheusCollector{
		metricsStore: ms,
		pipeline:     p,
		requestRate:  prometheus.NewDesc("tracing_request_rate", "Requests per second for service/operation", labels, nil),
		errorRate:    prometheus.NewDesc("tracing_error_rate_ratio", "Error ratio (0–1) for service/operation", labels, nil),
		latencyP50:   prometheus.NewDesc("tracing_latency_p50_ms", "P50 latency in milliseconds", labels, nil),
		latencyP95:   prometheus.NewDesc("tracing_latency_p95_ms", "P95 latency in milliseconds", labels, nil),
		latencyP99:   prometheus.NewDesc("tracing_latency_p99_ms", "P99 latency in milliseconds", labels, nil),
		sampledTotal: prometheus.NewDesc("tracing_sampled_spans_total", "Total spans accepted by sampler", nil, nil),
		droppedTotal: prometheus.NewDesc("tracing_dropped_spans_total", "Total spans dropped by sampler", nil, nil),
	}
}

func (c *prometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.requestRate
	ch <- c.errorRate
	ch <- c.latencyP50
	ch <- c.latencyP95
	ch <- c.latencyP99
	ch <- c.sampledTotal
	ch <- c.droppedTotal
}

func (c *prometheusCollector) Collect(ch chan<- prometheus.Metric) {
	for _, s := range c.metricsStore.Snapshot() {
		svc, op := s.Service, s.Operation
		ch <- prometheus.MustNewConstMetric(c.requestRate, prometheus.GaugeValue, s.Rate, svc, op)
		ch <- prometheus.MustNewConstMetric(c.errorRate, prometheus.GaugeValue, s.ErrorRate, svc, op)
		ch <- prometheus.MustNewConstMetric(c.latencyP50, prometheus.GaugeValue, s.P50Ms, svc, op)
		ch <- prometheus.MustNewConstMetric(c.latencyP95, prometheus.GaugeValue, s.P95Ms, svc, op)
		ch <- prometheus.MustNewConstMetric(c.latencyP99, prometheus.GaugeValue, s.P99Ms, svc, op)
	}

	sampled, dropped := c.pipeline.Stats()
	ch <- prometheus.MustNewConstMetric(c.sampledTotal, prometheus.CounterValue, float64(sampled))
	ch <- prometheus.MustNewConstMetric(c.droppedTotal, prometheus.CounterValue, float64(dropped))
}

// NewPrometheusHandler returns an http.Handler that serves Prometheus metrics.
func NewPrometheusHandler(ms *metrics.MetricsStore, p *Pipeline) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		newPrometheusCollector(ms, p),
	)
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
