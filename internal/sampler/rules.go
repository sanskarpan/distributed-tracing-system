package sampler

import (
	"sort"
	"strings"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// Rule defines a matching condition and its sampling decision.
type Rule struct {
	ServiceName   string
	OperationGlob string
	MinDuration   time.Duration
	StatusCode    *model.StatusCode
	Tags          map[string]string
	Decision      SamplingDecision
	Sampler       Sampler // if non-nil, delegate to this sampler instead of using Decision
	Priority      int
}

func (r *Rule) matches(p SamplingParameters) bool {
	if r.ServiceName != "" && r.ServiceName != p.ServiceName {
		return false
	}
	if r.OperationGlob != "" {
		if !globMatch(r.OperationGlob, p.OperationName) {
			return false
		}
	}
	for k, v := range r.Tags {
		found := false
		for _, attr := range p.Attributes {
			if attr.Key == k && attr.SVal == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// globMatch supports * as a wildcard matching any sequence of characters (including /).
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// RuleBasedSampler evaluates rules in priority order, falling back to a default sampler.
type RuleBasedSampler struct {
	rules    []Rule
	fallback Sampler
}

// NewRuleBased creates a RuleBasedSampler with the given rules and fallback.
func NewRuleBased(rules []Rule, fallback Sampler) *RuleBasedSampler {
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})
	return &RuleBasedSampler{rules: sorted, fallback: fallback}
}

func (s *RuleBasedSampler) ShouldSample(p SamplingParameters) SamplingResult {
	for _, rule := range s.rules {
		if rule.matches(p) {
			if rule.Sampler != nil {
				return rule.Sampler.ShouldSample(p)
			}
			return SamplingResult{Decision: rule.Decision, Reason: "rule matched"}
		}
	}
	return s.fallback.ShouldSample(p)
}

func (s *RuleBasedSampler) Name() string { return "rules" }
func (s *RuleBasedSampler) Config() map[string]any {
	return map[string]any{"ruleCount": len(s.rules), "fallback": s.fallback.Name()}
}
