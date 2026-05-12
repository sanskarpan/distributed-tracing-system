import type { SpanDetailDTO, TraceDetailDTO } from '@/types'

export interface SpanRow {
  spanId: string
  name: string
  serviceName: string
  startMs: number
  durationMs: number
  depth: number
  hasError: boolean
  kind: number
  span: SpanDetailDTO
}

export function buildSpanRows(trace: TraceDetailDTO): SpanRow[] {
  const rows: SpanRow[] = []
  const traceStart = Math.min(...trace.spans.map((span) => span.startTimeUnixNano)) / 1e6

  function visit(span: SpanDetailDTO, depth: number) {
    rows.push({
      spanId: span.spanId,
      name: span.name,
      serviceName: span.serviceName,
      startMs: span.startTimeUnixNano / 1e6 - traceStart,
      durationMs: span.durationMs,
      depth,
      hasError: span.status.code === 2,
      kind: span.kind,
      span,
    })

    const children = trace.spans
      .filter((candidate) => candidate.parentSpanId === span.spanId)
      .sort((left, right) => left.startTimeUnixNano - right.startTimeUnixNano)

    for (const child of children) {
      visit(child, depth + 1)
    }
  }

  const root = trace.spans.find(
    (span) => !span.parentSpanId || span.parentSpanId === '0000000000000000'
  )

  if (root) {
    visit(root, 0)
  }

  const visited = new Set(rows.map((row) => row.spanId))
  for (const span of trace.spans) {
    if (!visited.has(span.spanId)) {
      rows.push({
        spanId: span.spanId,
        name: span.name,
        serviceName: span.serviceName,
        startMs: span.startTimeUnixNano / 1e6 - traceStart,
        durationMs: span.durationMs,
        depth: 0,
        hasError: span.status.code === 2,
        kind: span.kind,
        span,
      })
    }
  }

  return rows
}
