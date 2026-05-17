package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yourname/tracing/internal/model"
)

const replicationHeader = "X-Trace-Replication"

type Replicator struct {
	peers      []string
	apiToken   string
	httpClient *http.Client

	successes int64
	failures  int64
	inFlight  int64
}

func NewReplicatorFromEnv() *Replicator {
	rawPeers := strings.TrimSpace(os.Getenv("REPLICA_PEERS"))
	if rawPeers == "" {
		return nil
	}
	peers := make([]string, 0)
	for _, peer := range strings.Split(rawPeers, ",") {
		peer = strings.TrimSpace(peer)
		if peer != "" {
			peers = append(peers, strings.TrimRight(peer, "/"))
		}
	}
	if len(peers) == 0 {
		return nil
	}
	return &Replicator{
		peers:    peers,
		apiToken: strings.TrimSpace(os.Getenv("REPLICA_API_TOKEN")),
		httpClient: &http.Client{
			Timeout: envDurationWithDefault("REPLICA_TIMEOUT", 5*time.Second),
		},
	}
}

func (r *Replicator) ReplicateAsync(spans []*model.Span, tenantID string) {
	if r == nil || len(spans) == 0 {
		return
	}
	body, err := json.Marshal(IngestRequest{Spans: spansToDTO(spans)})
	if err != nil {
		atomic.AddInt64(&r.failures, int64(len(r.peers)))
		return
	}

	for _, peer := range r.peers {
		peer := peer
		atomic.AddInt64(&r.inFlight, 1)
		go func() {
			defer atomic.AddInt64(&r.inFlight, -1)
			req, err := http.NewRequest(http.MethodPost, peer+"/api/v1/spans", bytes.NewReader(body))
			if err != nil {
				atomic.AddInt64(&r.failures, 1)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(replicationHeader, "1")
			if tenantID != "" {
				req.Header.Set("X-Tenant-ID", tenantID)
			}
			if r.apiToken != "" {
				req.Header.Set("Authorization", "Bearer "+r.apiToken)
			}
			resp, err := r.httpClient.Do(req)
			if err != nil {
				atomic.AddInt64(&r.failures, 1)
				return
			}
			resp.Body.Close()
			if resp.StatusCode >= 300 {
				atomic.AddInt64(&r.failures, 1)
				return
			}
			atomic.AddInt64(&r.successes, 1)
		}()
	}
}

func (r *Replicator) Stats() map[string]int64 {
	if r == nil {
		return map[string]int64{
			"replicationSuccesses": 0,
			"replicationFailures":  0,
			"replicationInFlight":  0,
			"replicationPeers":     0,
		}
	}
	return map[string]int64{
		"replicationSuccesses": atomic.LoadInt64(&r.successes),
		"replicationFailures":  atomic.LoadInt64(&r.failures),
		"replicationInFlight":  atomic.LoadInt64(&r.inFlight),
		"replicationPeers":     int64(len(r.peers)),
	}
}

func spansToDTO(spans []*model.Span) []SpanDTO {
	out := make([]SpanDTO, 0, len(spans))
	for _, sp := range spans {
		parentID := ""
		if !sp.ParentSpanID.IsZero() {
			parentID = sp.ParentSpanID.String()
		}
		dto := SpanDTO{
			TraceID:           sp.TraceID.String(),
			SpanID:            sp.SpanID.String(),
			ParentSpanID:      parentID,
			Name:              sp.Name,
			Kind:              int(sp.Kind),
			ServiceName:       sp.ServiceName,
			ServiceAttributes: sp.ServiceAttrs,
			StartTimeUnixNano: uint64(sp.StartTime.UnixNano()),
			EndTimeUnixNano:   uint64(sp.EndTime.UnixNano()),
			Attributes:        attributesToDTO(sp.Attributes),
			Status:            StatusDTO{Code: int(sp.Status.Code), Message: sp.Status.Message},
		}
		for _, event := range sp.Events {
			dto.Events = append(dto.Events, EventDTO{
				TimeUnixNano: uint64(event.Time.UnixNano()),
				Name:         event.Name,
				Attributes:   attributesToDTO(event.Attributes),
			})
		}
		for _, link := range sp.Links {
			dto.Links = append(dto.Links, LinkDTO{
				TraceID:    link.TraceID.String(),
				SpanID:     link.SpanID.String(),
				TraceState: link.TraceState,
				Attributes: attributesToDTO(link.Attributes),
			})
		}
		out = append(out, dto)
	}
	return out
}
