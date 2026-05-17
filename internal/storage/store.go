package storage

import (
	"time"

	"github.com/yourname/tracing/internal/model"
)

// TraceQuery specifies filter, sort, and pagination parameters for trace queries.
type TraceQuery struct {
	TenantID      string
	ServiceName   string
	OperationName string
	Tags          map[string]string
	AttributeKV   string // "key=value" attribute filter applied across all spans
	MinDuration   *time.Duration
	MaxDuration   *time.Duration
	HasError      *bool
	StartTime     *time.Time
	EndTime       *time.Time
	TraceID       *model.TraceID
	Limit         int
	Offset        int
	SortBy        string // "duration" | "receivedAt" | "spanCount"
	SortDesc      bool
}

// TraceQueryResult holds the paginated result of a query.
type TraceQueryResult struct {
	Traces  []*TraceSummary
	Total   int
	HasMore bool
}

// TraceSummary is a lightweight representation of a trace for listing/search results.
type TraceSummary struct {
	TenantID    string
	TraceID     model.TraceID
	RootService string
	RootOp      string
	Duration    time.Duration
	SpanCount   int
	Services    []string
	HasError    bool
	ReceivedAt  time.Time
}

// StoreStats holds operational statistics for the store.
type StoreStats struct {
	TraceCount int
	MaxSize    int
}

// TraceStore is the interface for persisting and querying traces.
type TraceStore interface {
	Upsert(trace *model.Trace) error
	Get(id model.TraceID) (*model.Trace, bool)
	Query(q *TraceQuery) (*TraceQueryResult, error)
	List(q *TraceQuery) ([]*model.Trace, error)
	Delete(id model.TraceID) error
	Services(tenantID string) []string
	Operations(service, tenantID string) []string
	Stats() StoreStats
}
