package api

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/tracing/internal/analysis"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/processor"
	"github.com/yourname/tracing/internal/sampler"
	"github.com/yourname/tracing/internal/storage"
)

type Pipeline struct {
	mu        sync.RWMutex
	sampler   sampler.Sampler
	assembler *processor.Assembler
	enricher  *processor.Enricher
	store     storage.TraceStore
	metrics   *metrics.MetricsStore
	sseBus    *SSEBus
	analyzer  *analysis.Analyzer

	// Worker pool
	workCh chan *model.Span
	wg     sync.WaitGroup

	shutdownOnce sync.Once

	// Stats
	sampledTotal int64
	droppedTotal int64
}

// workerCount is the number of parallel span-processing goroutines.
const workerCount = 4

// workerQueueDepth is the capacity of the span work queue.
const workerQueueDepth = 1024

func NewPipeline(store storage.TraceStore, metricsStore *metrics.MetricsStore,
	sseBus *SSEBus, s sampler.Sampler, analyzer *analysis.Analyzer, assemblerTimeout ...time.Duration) *Pipeline {

	timeout := 2 * time.Second
	if len(assemblerTimeout) > 0 && assemblerTimeout[0] > 0 {
		timeout = assemblerTimeout[0]
	}

	p := &Pipeline{
		sampler:  s,
		enricher: &processor.Enricher{},
		store:    store,
		metrics:  metricsStore,
		sseBus:   sseBus,
		analyzer: analyzer,
		workCh:   make(chan *model.Span, workerQueueDepth),
	}

	p.assembler = processor.NewAssembler(timeout, func(trace *model.Trace) {
		// Run analysis
		trace.CriticalPath = analyzer.ComputeCriticalPath(trace)
		if trace.RootSpan != nil {
			trace.ParallelGroups = analyzer.DetectParallelGroups(trace.RootSpan)
		}
		trace.Gaps = analyzer.DetectGaps(trace)

		// Store
		store.Upsert(trace)

		// Broadcast SSE
		summary := traceToSummarySSE(trace)
		sseBus.Broadcast(SSEEvent{Type: "trace", Data: summary})
	})

	// Start worker pool
	for i := 0; i < workerCount; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for span := range p.workCh {
				p.processSpan(span)
			}
		}()
	}

	return p
}

// StartWithContext wires pipeline shutdown to context cancellation.
func (p *Pipeline) StartWithContext(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = p.Shutdown(context.Background())
	}()
}

// IngestSpans validates, samples, enriches, and assembles spans.
func (p *Pipeline) IngestSpans(spans []*model.Span) (accepted, dropped int, err error) {
	for _, span := range spans {
		// Validate
		if span.TraceID.IsZero() {
			dropped++
			continue
		}
		if span.SpanID.IsZero() {
			dropped++
			continue
		}

		// Sample
		p.mu.RLock()
		s := p.sampler
		p.mu.RUnlock()

		// Special case: TailSampler buffers spans for deferred decision
		if ts, ok := s.(*sampler.TailSampler); ok {
			ts.AddSpan(span)
			accepted++ // counted as buffered/accepted from client's perspective
			continue
		}

		result := s.ShouldSample(sampler.SamplingParameters{
			TraceID:       span.TraceID,
			SpanID:        span.SpanID,
			ParentSpanID:  span.ParentSpanID,
			OperationName: span.Name,
			ServiceName:   span.ServiceName,
			Kind:          span.Kind,
			Attributes:    span.Attributes,
		})

		if result.Decision == sampler.Drop {
			atomic.AddInt64(&p.droppedTotal, 1)
			dropped++
			continue
		}

		// Submit to worker pool; fall back to inline if queue full.
		select {
		case p.workCh <- span:
		default:
			p.processSpan(span)
		}
		accepted++
	}
	return
}

// processSpan enriches, records metrics, broadcasts, and assembles a single pre-sampled span.
func (p *Pipeline) processSpan(span *model.Span) {
	p.enricher.Enrich(span)
	p.metrics.Record(span)
	p.sseBus.Broadcast(SSEEvent{Type: "span", Data: spanToSSE(span)})
	p.assembler.AddSpan(span)
	atomic.AddInt64(&p.sampledTotal, 1)
}

// SwapSampler atomically replaces the current sampler.
func (p *Pipeline) SwapSampler(s sampler.Sampler) {
	p.mu.Lock()
	old := p.sampler
	p.sampler = s
	p.mu.Unlock()

	if stopper, ok := old.(interface{ Stop() }); ok {
		stopper.Stop()
	}
}

// GetSampler returns the current sampler.
func (p *Pipeline) GetSampler() sampler.Sampler {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sampler
}

// Stats returns sampling statistics.
func (p *Pipeline) Stats() (sampled, dropped int64) {
	return atomic.LoadInt64(&p.sampledTotal), atomic.LoadInt64(&p.droppedTotal)
}

// QueueDepth returns the number of spans currently waiting in the worker pool queue.
func (p *Pipeline) QueueDepth() int {
	return len(p.workCh)
}

// Shutdown drains worker processing, flushes deferred sampler state,
// and finalizes traces still pending assembly.
func (p *Pipeline) Shutdown(ctx context.Context) error {
	var shutdownErr error

	p.shutdownOnce.Do(func() {
		if stopper, ok := p.GetSampler().(interface{ Stop() }); ok {
			stopper.Stop()
		}

		close(p.workCh)

		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			p.assembler.Stop()
		case <-ctx.Done():
			shutdownErr = ctx.Err()
		}
	})

	return shutdownErr
}

type spanSSE struct {
	TraceID    string  `json:"traceId"`
	SpanID     string  `json:"spanId"`
	Service    string  `json:"service"`
	Operation  string  `json:"operation"`
	DurationMs float64 `json:"durationMs"`
	HasError   bool    `json:"hasError"`
	Ts         string  `json:"ts"`
}

type traceSSE struct {
	TraceID     string    `json:"traceId"`
	DurationMs  float64   `json:"durationMs"`
	SpanCount   int       `json:"spanCount"`
	RootService string    `json:"rootService"`
	RootOp      string    `json:"rootOp"`
	Services    []string  `json:"services"`
	HasError    bool      `json:"hasError"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

func spanToSSE(sp *model.Span) spanSSE {
	var dur float64
	if !sp.StartTime.IsZero() && !sp.EndTime.IsZero() {
		dur = float64(sp.EndTime.Sub(sp.StartTime).Nanoseconds()) / 1e6
	}
	return spanSSE{
		TraceID:    sp.TraceID.String(),
		SpanID:     sp.SpanID.String(),
		Service:    sp.ServiceName,
		Operation:  sp.Name,
		DurationMs: dur,
		HasError:   sp.HasError,
		Ts:         time.Now().UTC().Format(time.RFC3339),
	}
}

func traceToSummarySSE(t *model.Trace) traceSSE {
	rootService := ""
	rootOp := ""
	if t.RootSpan != nil {
		rootService = t.RootSpan.ServiceName
		rootOp = t.RootSpan.Name
	}
	var hasError bool
	for _, sp := range t.Spans {
		if sp.HasError {
			hasError = true
			break
		}
	}
	return traceSSE{
		TraceID:     t.TraceID.String(),
		DurationMs:  float64(t.Duration.Nanoseconds()) / 1e6,
		SpanCount:   t.SpanCount,
		RootService: rootService,
		RootOp:      rootOp,
		Services:    t.Services,
		HasError:    hasError,
		ReceivedAt:  t.ReceivedAt,
	}
}
