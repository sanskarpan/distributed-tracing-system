package api

import (
	"time"

	"github.com/yourname/tracing/internal/model"
)

// SpanDTO is the native JSON format for ingesting spans.
type SpanDTO struct {
	TraceID           string            `json:"traceId"`
	SpanID            string            `json:"spanId"`
	ParentSpanID      string            `json:"parentSpanId"`
	Name              string            `json:"name"`
	Kind              int               `json:"kind"`
	ServiceName       string            `json:"serviceName"`
	ServiceAttributes map[string]string `json:"serviceAttributes"`
	StartTimeUnixNano uint64            `json:"startTimeUnixNano"`
	EndTimeUnixNano   uint64            `json:"endTimeUnixNano"`
	Attributes        []AttributeDTO    `json:"attributes"`
	Events            []EventDTO        `json:"events"`
	Links             []LinkDTO         `json:"links"`
	Status            StatusDTO         `json:"status"`
}

type AttributeDTO struct {
	Key         string   `json:"key"`
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

type EventDTO struct {
	TimeUnixNano uint64         `json:"timeUnixNano"`
	Name         string         `json:"name"`
	Attributes   []AttributeDTO `json:"attributes"`
}

type LinkDTO struct {
	TraceID    string         `json:"traceId"`
	SpanID     string         `json:"spanId"`
	TraceState string         `json:"traceState"`
	Attributes []AttributeDTO `json:"attributes"`
}

type StatusDTO struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// IngestRequest is the native batch ingestion format.
type IngestRequest struct {
	Spans []SpanDTO `json:"spans"`
}

// IngestResponse is the response to a native ingest request.
type IngestResponse struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}

// SpanDetailDTO is the response format for a single span in a trace detail.
type SpanDetailDTO struct {
	SpanID            string         `json:"spanId"`
	ParentSpanID      string         `json:"parentSpanId"`
	TraceID           string         `json:"traceId"`
	Name              string         `json:"name"`
	ServiceName       string         `json:"serviceName"`
	Kind              int            `json:"kind"`
	StartTimeUnixNano uint64         `json:"startTimeUnixNano"`
	DurationMs        float64        `json:"durationMs"`
	Status            StatusDTO      `json:"status"`
	Attributes        []AttributeDTO `json:"attributes"`
	Events            []SpanEventDTO `json:"events"`
	Links             []LinkDTO      `json:"links"`
	Depth             int            `json:"depth"`
	HasError          bool           `json:"hasError"`
}

type SpanEventDTO struct {
	TimeUnixNano uint64         `json:"timeUnixNano"`
	Name         string         `json:"name"`
	Attributes   []AttributeDTO `json:"attributes"`
}

// TraceDetailDTO is the response format for a full trace.
type TraceDetailDTO struct {
	TraceID        string             `json:"traceId"`
	Spans          []SpanDetailDTO    `json:"spans"`
	CriticalPath   []string           `json:"criticalPath"` // SpanIDs
	Services       []string           `json:"services"`
	DurationMs     float64            `json:"durationMs"`
	SpanCount      int                `json:"spanCount"`
	ErrorCount     int                `json:"errorCount"`
	ParallelGroups []ParallelGroupDTO `json:"parallelGroups"`
	Gaps           []SpanGapDTO       `json:"gaps"`
}

type ParallelGroupDTO struct {
	SpanIDs []string `json:"spanIds"`
	StartMs float64  `json:"startMs"`
	EndMs   float64  `json:"endMs"`
}

type SpanGapDTO struct {
	BeforeSpanID string  `json:"beforeSpanId"`
	AfterSpanID  string  `json:"afterSpanId"`
	DurationMs   float64 `json:"durationMs"`
}

// TraceSummaryDTO is the compact representation for list views.
type TraceSummaryDTO struct {
	TraceID     string    `json:"traceId"`
	RootService string    `json:"rootService"`
	RootOp      string    `json:"rootOp"`
	DurationMs  float64   `json:"durationMs"`
	SpanCount   int       `json:"spanCount"`
	Services    []string  `json:"services"`
	HasError    bool      `json:"hasError"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

// SamplerConfigRequest is the body for PUT /api/v1/sampler.
type SamplerConfigRequest struct {
	Type             string          `json:"type"`
	Rate             *float64        `json:"rate,omitempty"`
	TracesPerSec     *float64        `json:"tracesPerSec,omitempty"`
	TargetRate       *float64        `json:"targetRate,omitempty"`
	MinRate          *float64        `json:"minRate,omitempty"`
	MaxRate          *float64        `json:"maxRate,omitempty"`
	BufferTimeoutSec *float64        `json:"bufferTimeoutSec,omitempty"`
	Policies         []TailPolicyDTO `json:"policies,omitempty"`
	Rules            []RuleDTO       `json:"rules,omitempty"`
}

// RuleDTO represents a single rule in a rule-based sampler config.
type RuleDTO struct {
	OperationGlob string      `json:"operationGlob"`
	ServiceName   string      `json:"serviceName"`
	Priority      int         `json:"priority"`
	Sampler       *SamplerDTO `json:"sampler,omitempty"`
}

// SamplerDTO is a nested sampler reference inside a rule.
type SamplerDTO struct {
	Type string   `json:"type"`
	Rate *float64 `json:"rate,omitempty"`
}

type TailPolicyDTO struct {
	Type        string   `json:"type"`
	ThresholdMs *float64 `json:"thresholdMs,omitempty"`
	Rate        *float64 `json:"rate,omitempty"`
}

// Helper functions for converting model types to DTOs
func attributesToDTO(kvs []model.KeyValue) []AttributeDTO {
	out := make([]AttributeDTO, 0, len(kvs))
	for _, kv := range kvs {
		dto := AttributeDTO{Key: kv.Key}
		switch kv.Type {
		case model.ValueString:
			s := kv.SVal
			dto.StringValue = &s
		case model.ValueInt:
			v := kv.IVal
			dto.IntValue = &v
		case model.ValueBool:
			b := kv.BVal
			dto.BoolValue = &b
		case model.ValueFloat:
			f := kv.FVal
			dto.DoubleValue = &f
		}
		out = append(out, dto)
	}
	return out
}

func dtoToAttributes(dtos []AttributeDTO) []model.KeyValue {
	out := make([]model.KeyValue, 0, len(dtos))
	for _, dto := range dtos {
		if dto.StringValue != nil {
			out = append(out, model.StringKV(dto.Key, *dto.StringValue))
		} else if dto.IntValue != nil {
			out = append(out, model.IntKV(dto.Key, *dto.IntValue))
		} else if dto.BoolValue != nil {
			out = append(out, model.BoolKV(dto.Key, *dto.BoolValue))
		} else if dto.DoubleValue != nil {
			out = append(out, model.FloatKV(dto.Key, *dto.DoubleValue))
		}
	}
	return out
}

func spanToDetailDTO(sp *model.Span, traceStartNano uint64) SpanDetailDTO {
	var durationMs float64
	if !sp.StartTime.IsZero() && !sp.EndTime.IsZero() {
		durationMs = float64(sp.EndTime.Sub(sp.StartTime).Nanoseconds()) / 1e6
	}

	events := make([]SpanEventDTO, 0, len(sp.Events))
	for _, e := range sp.Events {
		events = append(events, SpanEventDTO{
			TimeUnixNano: uint64(e.Time.UnixNano()),
			Name:         e.Name,
			Attributes:   attributesToDTO(e.Attributes),
		})
	}

	links := make([]LinkDTO, 0, len(sp.Links))
	for _, l := range sp.Links {
		links = append(links, LinkDTO{
			TraceID:    l.TraceID.String(),
			SpanID:     l.SpanID.String(),
			TraceState: l.TraceState,
			Attributes: attributesToDTO(l.Attributes),
		})
	}

	parentSpanID := ""
	if !sp.ParentSpanID.IsZero() {
		parentSpanID = sp.ParentSpanID.String()
	}

	return SpanDetailDTO{
		SpanID:            sp.SpanID.String(),
		ParentSpanID:      parentSpanID,
		TraceID:           sp.TraceID.String(),
		Name:              sp.Name,
		ServiceName:       sp.ServiceName,
		Kind:              int(sp.Kind),
		StartTimeUnixNano: uint64(sp.StartTime.UnixNano()),
		DurationMs:        durationMs,
		Status:            StatusDTO{Code: int(sp.Status.Code), Message: sp.Status.Message},
		Attributes:        attributesToDTO(sp.Attributes),
		Events:            events,
		Links:             links,
		Depth:             sp.Depth,
		HasError:          sp.HasError,
	}
}
