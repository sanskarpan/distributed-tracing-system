package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/yourname/tracing/internal/model"
	"github.com/yourname/tracing/internal/sampler"
)

type SamplerHandler struct {
	pipeline *Pipeline
}

func NewSamplerHandler(pipeline *Pipeline) *SamplerHandler {
	return &SamplerHandler{pipeline: pipeline}
}

// HandleGetSampler handles GET /api/v1/sampler
func (h *SamplerHandler) HandleGetSampler(w http.ResponseWriter, r *http.Request) {
	s := h.pipeline.GetSampler()
	sampledTotal, droppedTotal := h.pipeline.Stats()
	total := sampledTotal + droppedTotal
	var rate float64
	if total > 0 {
		rate = float64(sampledTotal) / float64(total)
	}
	writeJSON(w, map[string]any{
		"type":   s.Name(),
		"config": s.Config(),
		"stats": map[string]any{
			"sampledTotal": sampledTotal,
			"droppedTotal": droppedTotal,
			"samplingRate": rate,
		},
	})
}

// HandlePutSampler handles PUT /api/v1/sampler
func (h *SamplerHandler) HandlePutSampler(w http.ResponseWriter, r *http.Request) {
	var req SamplerConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, 400)
		return
	}

	s, err := h.buildSampler(req)
	if err != nil {
		msg, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(msg), 400)
		return
	}

	h.pipeline.SwapSampler(s)
	writeJSON(w, map[string]any{"ok": true, "type": req.Type})
}

func (h *SamplerHandler) buildSampler(req SamplerConfigRequest) (sampler.Sampler, error) {
	switch req.Type {
	case "always":
		return sampler.NewAlways(), nil
	case "never":
		return sampler.NewNever(), nil
	case "probabilistic":
		rate := 0.1
		if req.Rate != nil {
			rate = *req.Rate
		}
		if err := validateRate(rate, "rate"); err != nil {
			return nil, err
		}
		return sampler.NewProbabilistic(rate), nil
	case "ratelimit":
		tps := 100.0
		if req.TracesPerSec != nil {
			tps = *req.TracesPerSec
		}
		if tps <= 0 {
			return nil, fmt.Errorf("tracesPerSec must be > 0")
		}
		return sampler.NewRateLimit(tps), nil
	case "adaptive":
		target := 100.0
		minRate := 0.001
		maxRate := 1.0
		if req.TargetRate != nil {
			target = *req.TargetRate
		}
		if req.MinRate != nil {
			minRate = *req.MinRate
		}
		if req.MaxRate != nil {
			maxRate = *req.MaxRate
		}
		if target <= 0 {
			return nil, fmt.Errorf("targetRate must be > 0")
		}
		if err := validateRate(minRate, "minRate"); err != nil {
			return nil, err
		}
		if err := validateRate(maxRate, "maxRate"); err != nil {
			return nil, err
		}
		if minRate > maxRate {
			return nil, fmt.Errorf("minRate must be <= maxRate")
		}
		return sampler.NewAdaptive(target, minRate, maxRate, 5*time.Second), nil
	case "rules":
		rules := make([]sampler.Rule, 0, len(req.Rules))
		for _, r := range req.Rules {
			rule := sampler.Rule{
				ServiceName:   r.ServiceName,
				OperationGlob: r.OperationGlob,
				Priority:      r.Priority,
			}
			if r.Sampler != nil {
				switch r.Sampler.Type {
				case "always":
					rule.Sampler = sampler.NewAlways()
				case "never":
					rule.Sampler = sampler.NewNever()
				case "probabilistic":
					rate := 0.1
					if r.Sampler.Rate != nil {
						rate = *r.Sampler.Rate
					}
					if err := validateRate(rate, "rules[].sampler.rate"); err != nil {
						return nil, err
					}
					rule.Sampler = sampler.NewProbabilistic(rate)
				default:
					return nil, fmt.Errorf("unknown rules[].sampler.type %q", r.Sampler.Type)
				}
			} else {
				rule.Decision = sampler.Sample
			}
			rules = append(rules, rule)
		}
		return sampler.NewRuleBased(rules, sampler.NewAlways()), nil
	case "tail":
		timeout := 10.0
		if req.BufferTimeoutSec != nil {
			timeout = *req.BufferTimeoutSec
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("bufferTimeoutSec must be > 0")
		}
		policies, err := buildTailPolicies(req.Policies)
		if err != nil {
			return nil, err
		}
		pipeline := h.pipeline
		accept := func(spans []*model.Span) {
			for _, sp := range spans {
				pipeline.processSpan(sp)
			}
		}
		reject := func(id model.TraceID) {
			atomic.AddInt64(&pipeline.droppedTotal, 1)
		}
		return sampler.NewTailSampler(
			time.Duration(timeout*float64(time.Second)),
			10000, policies, accept, reject,
		), nil
	default:
		return nil, fmt.Errorf("unknown sampler type %q", req.Type)
	}
}

func buildTailPolicies(dtos []TailPolicyDTO) ([]sampler.TailPolicy, error) {
	policies := make([]sampler.TailPolicy, 0, len(dtos))
	for _, dto := range dtos {
		switch dto.Type {
		case "error":
			policies = append(policies, sampler.ErrorPolicy{})
		case "latency":
			threshold := 500.0
			if dto.ThresholdMs != nil {
				threshold = *dto.ThresholdMs
			}
			if threshold <= 0 {
				return nil, fmt.Errorf("tail latency thresholdMs must be > 0")
			}
			policies = append(policies, sampler.LatencyPolicy{
				Threshold: time.Duration(threshold * float64(time.Millisecond)),
			})
		case "probabilistic":
			rate := 0.1
			if dto.Rate != nil {
				rate = *dto.Rate
			}
			if err := validateRate(rate, "tail probabilistic rate"); err != nil {
				return nil, err
			}
			policies = append(policies, sampler.NewProbabilisticTailPolicy(rate))
		default:
			return nil, fmt.Errorf("unknown tail policy type %q", dto.Type)
		}
	}
	return policies, nil
}

func validateRate(v float64, field string) error {
	if v < 0 || v > 1 {
		return fmt.Errorf("%s must be between 0 and 1", field)
	}
	return nil
}
