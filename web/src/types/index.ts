export interface TraceID { value: string }
export interface SpanID { value: string }

export interface AttributeDTO {
  key: string
  stringValue?: string
  intValue?: number
  boolValue?: boolean
  doubleValue?: number
}

export interface SpanEventDTO {
  timeUnixNano: number
  name: string
  attributes: AttributeDTO[]
}

export interface LinkDTO {
  traceId: string
  spanId: string
  traceState: string
  attributes: AttributeDTO[]
}

export interface StatusDTO {
  code: number  // 0=unset, 1=ok, 2=error
  message: string
}

export interface SpanDetailDTO {
  spanId: string
  parentSpanId: string
  traceId: string
  name: string
  serviceName: string
  kind: number  // 1=internal, 2=server, 3=client, 4=producer, 5=consumer
  startTimeUnixNano: number
  durationMs: number
  status: StatusDTO
  attributes: AttributeDTO[]
  events: SpanEventDTO[]
  links: LinkDTO[]
  depth: number
  hasError: boolean
}

export interface ParallelGroupDTO {
  spanIds: string[]
  startMs: number
  endMs: number
}

export interface SpanGapDTO {
  beforeSpanId: string
  afterSpanId: string
  durationMs: number
}

export interface TraceDetailDTO {
  traceId: string
  spans: SpanDetailDTO[]
  criticalPath: string[]  // spanIds
  services: string[]
  durationMs: number
  spanCount: number
  errorCount: number
  parallelGroups: ParallelGroupDTO[]
  gaps: SpanGapDTO[]
}

export interface TraceSummaryDTO {
  traceId: string
  rootService: string
  rootOp: string
  durationMs: number
  spanCount: number
  services: string[]
  hasError: boolean
  receivedAt: string
}

export interface TraceListResponse {
  traces: TraceSummaryDTO[]
  total: number
  hasMore: boolean
}

export interface MetricSnapshotDTO {
  service: string
  operation: string
  rate: number
  errorRate: number
  p50Ms: number
  p95Ms: number
  p99Ms: number
}

export interface ServiceNode {
  name: string
  spanCount: number
  errorRate: number
  p99Ms: number
  reqPerSec: number
}

export interface ServiceEdge {
  caller: string
  callee: string
  count: number
  errorCount: number
  p99Ms: number
}

export interface DependencyGraph {
  services: ServiceNode[]
  edges: ServiceEdge[]
}

export interface SamplerStats {
  sampledTotal: number
  droppedTotal: number
  samplingRate: number
}

export interface SamplerConfig {
  type: string
  config: Record<string, unknown>
  stats: SamplerStats
}

export interface TraceComparisonDTO {
  durationDeltaMs: number
  spanCountDelta: number
  errorDelta: number
  matched: Array<{ baseSpanId: string; compareSpanId: string; durationDeltaMs: number }>
  onlyInBase: string[]
  onlyInCompare: string[]
}
