import { describe, expect, it } from 'vitest'
import { hasActiveSearchFilters, mergeUniqueTraces } from './search-helpers'
import type { TraceSummaryDTO } from '@/types'

function makeTrace(traceId: string): TraceSummaryDTO {
  return {
    traceId,
    rootService: 'svc',
    rootOp: 'op',
    durationMs: 10,
    spanCount: 1,
    services: ['svc'],
    hasError: false,
    receivedAt: new Date(0).toISOString(),
  }
}

describe('Search helpers', () => {
  it('detects when user-facing filters are active', () => {
    expect(hasActiveSearchFilters({ limit: 20, offset: 0 })).toBe(false)
    expect(hasActiveSearchFilters({ service: 'checkout' })).toBe(true)
    expect(hasActiveSearchFilters({ status: 'error' })).toBe(true)
    expect(hasActiveSearchFilters({ timeRangeMinutes: 15 })).toBe(true)
  })

  it('deduplicates traces across live and queried groups by traceId', () => {
    const a = makeTrace('trace-a')
    const b = makeTrace('trace-b')
    const duplicateA = { ...a, durationMs: 20 }

    const merged = mergeUniqueTraces([a, b], [duplicateA])

    expect(merged).toHaveLength(2)
    expect(merged[0].traceId).toBe('trace-a')
    expect(merged[1].traceId).toBe('trace-b')
  })
})
