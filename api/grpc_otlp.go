package api

import (
	"context"
	"encoding/hex"
	"time"

	coltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/yourname/tracing/internal/model"
)

// OTLPTraceServiceServer implements the OpenTelemetry OTLP gRPC TraceService.
type OTLPTraceServiceServer struct {
	coltracev1.UnimplementedTraceServiceServer
	pipeline *Pipeline
}

// NewOTLPTraceServiceServer creates a new gRPC trace receiver backed by the pipeline.
func NewOTLPTraceServiceServer(pipeline *Pipeline) *OTLPTraceServiceServer {
	return &OTLPTraceServiceServer{pipeline: pipeline}
}

// Export implements TraceServiceServer.Export.
// It converts OTLP protobuf spans to the internal model and feeds them into the pipeline.
func (s *OTLPTraceServiceServer) Export(
	_ context.Context,
	req *coltracev1.ExportTraceServiceRequest,
) (*coltracev1.ExportTraceServiceResponse, error) {
	spans := otlpProtoToSpans(req.GetResourceSpans())
	if len(spans) > 0 {
		s.pipeline.IngestSpans(spans)
	}
	return &coltracev1.ExportTraceServiceResponse{}, nil
}

func otlpProtoToSpans(resourceSpans []*tracev1.ResourceSpans) []*model.Span {
	var spans []*model.Span
	for _, rs := range resourceSpans {
		serviceName := "unknown"
		if rs.Resource != nil {
			for _, kv := range rs.Resource.Attributes {
				if kv.Key == "service.name" {
					if sv := kv.Value.GetStringValue(); sv != "" {
						serviceName = sv
					}
				}
			}
		}

		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				span := protoSpanToModel(sp, serviceName)
				if span != nil {
					spans = append(spans, span)
				}
			}
		}
	}
	return spans
}

func protoSpanToModel(sp *tracev1.Span, serviceName string) *model.Span {
	if sp == nil {
		return nil
	}

	traceIDHex := hex.EncodeToString(sp.TraceId)
	spanIDHex := hex.EncodeToString(sp.SpanId)

	traceID, err := model.ParseTraceID(traceIDHex)
	if err != nil {
		return nil
	}
	spanID, err := model.ParseSpanID(spanIDHex)
	if err != nil {
		return nil
	}

	ms := &model.Span{
		TraceID:     traceID,
		SpanID:      spanID,
		Name:        sp.Name,
		ServiceName: serviceName,
		Kind:        model.SpanKind(sp.Kind),
		StartTime:   time.Unix(0, int64(sp.StartTimeUnixNano)),
		EndTime:     time.Unix(0, int64(sp.EndTimeUnixNano)),
	}

	if len(sp.ParentSpanId) > 0 {
		parentHex := hex.EncodeToString(sp.ParentSpanId)
		if parentID, err := model.ParseSpanID(parentHex); err == nil {
			ms.ParentSpanID = parentID
		}
	}

	for _, kv := range sp.Attributes {
		ms.Attributes = append(ms.Attributes, protoKVToModel(kv))
	}

	if sp.Status != nil {
		ms.Status.Code = model.StatusCode(sp.Status.Code)
		ms.Status.Message = sp.Status.Message
		ms.HasError = sp.Status.Code == tracev1.Status_STATUS_CODE_ERROR
	}

	return ms
}

func protoKVToModel(kv *commonv1.KeyValue) model.KeyValue {
	if kv == nil || kv.Value == nil {
		return model.KeyValue{}
	}
	mk := model.KeyValue{Key: kv.Key}
	switch v := kv.Value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		mk.Type = model.ValueString
		mk.SVal = v.StringValue
	case *commonv1.AnyValue_IntValue:
		mk.Type = model.ValueInt
		mk.IVal = v.IntValue
	case *commonv1.AnyValue_BoolValue:
		mk.Type = model.ValueBool
		mk.BVal = v.BoolValue
	case *commonv1.AnyValue_DoubleValue:
		mk.Type = model.ValueFloat
		mk.FVal = v.DoubleValue
	}
	return mk
}

