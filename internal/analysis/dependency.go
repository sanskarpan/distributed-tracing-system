package analysis

import (
	"sort"
	"time"

	"github.com/yourname/tracing/internal/model"
)

// ServiceEdge represents a directed call relationship between two services.
type ServiceEdge struct {
	Caller     string  `json:"caller"`
	Callee     string  `json:"callee"`
	Count      int64   `json:"count"`
	ErrorCount int64   `json:"errorCount"`
	P99Ms      float64 `json:"p99Ms"`
	durations  []float64
}

// ServiceNode summarizes metrics for a single service.
type ServiceNode struct {
	Name      string  `json:"name"`
	SpanCount int64   `json:"spanCount"`
	ErrorRate float64 `json:"errorRate"`
	P99Ms     float64 `json:"p99Ms"`
	ReqPerSec float64 `json:"reqPerSec"`
}

// DependencyGraph is the result of dependency analysis over a set of traces.
type DependencyGraph struct {
	Services []ServiceNode
	Edges    []*ServiceEdge
}

// BuildDependencyGraph builds a service dependency graph from recent traces.
// For each CLIENT span: caller = span.ServiceName, callee = span.Attributes["peer.service"].
func (a *Analyzer) BuildDependencyGraph(traces []*model.Trace, since time.Time) *DependencyGraph {
	type edgeKey struct{ caller, callee string }
	edgeMap := make(map[edgeKey]*ServiceEdge)

	type nodeKey = string
	type nodeData struct {
		spanCount  int64
		errCount   int64
		durations  []float64
		firstSeen  time.Time
		lastSeen   time.Time
	}
	nodeMap := make(map[nodeKey]*nodeData)

	ensureNode := func(name string, t time.Time) *nodeData {
		nd := nodeMap[name]
		if nd == nil {
			nd = &nodeData{firstSeen: t, lastSeen: t}
			nodeMap[name] = nd
		}
		if t.Before(nd.firstSeen) {
			nd.firstSeen = t
		}
		if t.After(nd.lastSeen) {
			nd.lastSeen = t
		}
		return nd
	}

	for _, trace := range traces {
		for _, span := range trace.Spans {
			if span.StartTime.Before(since) {
				continue
			}

			// Update node stats for the service
			if span.ServiceName != "" {
				nd := ensureNode(span.ServiceName, span.StartTime)
				nd.spanCount++
				if span.Status.Code == model.StatusError {
					nd.errCount++
				}
				durMs := float64(span.Duration()) / float64(time.Millisecond)
				nd.durations = append(nd.durations, durMs)
			}

			// Build edges from CLIENT spans
			if span.Kind != model.SpanKindClient {
				continue
			}

			caller := span.ServiceName
			if caller == "" {
				continue
			}

			var callee string
			for _, kv := range span.Attributes {
				if kv.Key == "peer.service" {
					callee = kv.SVal
					break
				}
			}
			if callee == "" {
				continue
			}

			key := edgeKey{caller: caller, callee: callee}
			edge := edgeMap[key]
			if edge == nil {
				edge = &ServiceEdge{Caller: caller, Callee: callee}
				edgeMap[key] = edge
			}
			edge.Count++
			if span.Status.Code == model.StatusError {
				edge.ErrorCount++
			}
			durMs := float64(span.Duration()) / float64(time.Millisecond)
			edge.durations = append(edge.durations, durMs)
		}
	}

	// Compute P99 for edges
	edges := make([]*ServiceEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		if len(edge.durations) > 0 {
			sorted := make([]float64, len(edge.durations))
			copy(sorted, edge.durations)
			sort.Float64s(sorted)
			idx := int(0.99 * float64(len(sorted)-1))
			edge.P99Ms = sorted[idx]
		}
		edge.durations = nil // clear raw data
		edges = append(edges, edge)
	}

	// Sort edges for determinism
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Caller != edges[j].Caller {
			return edges[i].Caller < edges[j].Caller
		}
		return edges[i].Callee < edges[j].Callee
	})

	// Build service nodes
	nodeNames := make([]string, 0, len(nodeMap))
	for name := range nodeMap {
		nodeNames = append(nodeNames, name)
	}
	sort.Strings(nodeNames)

	nodes := make([]ServiceNode, 0, len(nodeMap))
	for _, name := range nodeNames {
		nd := nodeMap[name]
		var p99Ms float64
		if len(nd.durations) > 0 {
			sorted := make([]float64, len(nd.durations))
			copy(sorted, nd.durations)
			sort.Float64s(sorted)
			idx := int(0.99 * float64(len(sorted)-1))
			p99Ms = sorted[idx]
		}
		var errorRate float64
		if nd.spanCount > 0 {
			errorRate = float64(nd.errCount) / float64(nd.spanCount)
		}
		var reqPerSec float64
		windowSecs := nd.lastSeen.Sub(nd.firstSeen).Seconds()
		if windowSecs > 0 {
			reqPerSec = float64(nd.spanCount) / windowSecs
		}
		nodes = append(nodes, ServiceNode{
			Name:      name,
			SpanCount: nd.spanCount,
			ErrorRate: errorRate,
			P99Ms:     p99Ms,
			ReqPerSec: reqPerSec,
		})
	}

	return &DependencyGraph{
		Services: nodes,
		Edges:    edges,
	}
}
