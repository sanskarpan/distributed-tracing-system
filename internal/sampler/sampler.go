package sampler

import (
	"github.com/yourname/tracing/internal/model"
)

// SamplingDecision is the result of a sampling decision.
type SamplingDecision int

const (
	Drop       SamplingDecision = iota
	RecordOnly                  // record locally, don't export
	Sample                      // record and export
)

// SamplingParameters are the inputs to a sampling decision.
type SamplingParameters struct {
	TraceID       model.TraceID
	SpanID        model.SpanID
	ParentSpanID  model.SpanID
	OperationName string
	ServiceName   string
	Kind          model.SpanKind
	Attributes    []model.KeyValue
	ParentSampled *bool // nil = no parent info
}

// SamplingResult is the output of a sampling decision.
type SamplingResult struct {
	Decision   SamplingDecision
	Attributes []model.KeyValue
	Reason     string
}

// Sampler makes sampling decisions for spans.
type Sampler interface {
	ShouldSample(p SamplingParameters) SamplingResult
	Name() string
	Config() map[string]any
}
