import { describe, it, expect } from 'vitest'
import { buildSpanRows } from './WaterfallChart'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'

function makeSpan(overrides: Partial<SpanDetailDTO> & { spanId: string }): SpanDetailDTO {
  return {
    spanId: overrides.spanId,
    parentSpanId: overrides.parentSpanId ?? '',
    traceId: 'abc123',
    name: overrides.name ?? 'op',
    serviceName: overrides.serviceName ?? 'svc',
    kind: overrides.kind ?? 1,
    startTimeUnixNano: overrides.startTimeUnixNano ?? 1_000_000_000,
    durationMs: overrides.durationMs ?? 100,
    status: overrides.status ?? { code: 0, message: '' },
    attributes: [],
    events: [],
    links: [],
    depth: overrides.depth ?? 0,
    hasError: overrides.hasError ?? false,
  }
}

function makeTrace(spans: SpanDetailDTO[]): TraceDetailDTO {
  return {
    traceId: 'abc123',
    spans,
    criticalPath: [],
    services: [],
    durationMs: 500,
    spanCount: spans.length,
    errorCount: 0,
    parallelGroups: [],
    gaps: [],
  }
}

describe('buildSpanRows', () => {
  it('returns 1 row for a single-span trace', () => {
    const span = makeSpan({ spanId: 'aaaa1111' })
    const trace = makeTrace([span])
    const rows = buildSpanRows(trace)
    expect(rows).toHaveLength(1)
    expect(rows[0].spanId).toBe('aaaa1111')
  })

  it('returns rows in DFS order (parent before children)', () => {
    // Build: root → child1 → grandchild, root → child2
    const root = makeSpan({ spanId: 'root0001', startTimeUnixNano: 1000 })
    const child1 = makeSpan({ spanId: 'chld0001', parentSpanId: 'root0001', startTimeUnixNano: 1010 })
    const grandchild = makeSpan({ spanId: 'grnd0001', parentSpanId: 'chld0001', startTimeUnixNano: 1020 })
    const child2 = makeSpan({ spanId: 'chld0002', parentSpanId: 'root0001', startTimeUnixNano: 1030 })

    const trace = makeTrace([root, child1, grandchild, child2])
    const rows = buildSpanRows(trace)

    expect(rows).toHaveLength(4)
    // DFS: root, child1, grandchild, child2
    expect(rows[0].spanId).toBe('root0001')
    expect(rows[1].spanId).toBe('chld0001')
    expect(rows[2].spanId).toBe('grnd0001')
    expect(rows[3].spanId).toBe('chld0002')
  })

  it('marks error spans with hasError=true', () => {
    const root = makeSpan({ spanId: 'root0001' })
    const errSpan = makeSpan({
      spanId: 'err00001',
      parentSpanId: 'root0001',
      status: { code: 2, message: 'boom' },
    })
    const trace = makeTrace([root, errSpan])
    const rows = buildSpanRows(trace)

    const errRow = rows.find(r => r.spanId === 'err00001')
    expect(errRow).toBeDefined()
    expect(errRow!.hasError).toBe(true)

    const rootRow = rows.find(r => r.spanId === 'root0001')
    expect(rootRow!.hasError).toBe(false)
  })

  it('returns all spans including orphans (no parent)', () => {
    const root = makeSpan({ spanId: 'root0001' })
    const orphan = makeSpan({ spanId: 'orph0001', parentSpanId: 'missing1' })
    const trace = makeTrace([root, orphan])
    const rows = buildSpanRows(trace)
    expect(rows).toHaveLength(2)
    const ids = rows.map(r => r.spanId)
    expect(ids).toContain('root0001')
    expect(ids).toContain('orph0001')
  })

  it('span rows carry the spanId so criticalPath overlay can match them', () => {
    // The component uses criticalPathIds.has(row.spanId) to apply amber stroke.
    // Verify that spanId is populated so callers can build criticalPathIds correctly.
    const span = makeSpan({ spanId: 'crit0001' })
    const trace = makeTrace([span])
    const rows = buildSpanRows(trace)
    const criticalPathIds = new Set(['crit0001'])
    expect(criticalPathIds.has(rows[0].spanId)).toBe(true)
  })
})
