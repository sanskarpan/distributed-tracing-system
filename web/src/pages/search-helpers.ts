import type { SearchFilters } from '@/hooks/useSearch'
import type { TraceSummaryDTO } from '@/types'

export function hasActiveSearchFilters(filters: SearchFilters): boolean {
  return Boolean(
    filters.service ||
      filters.operation ||
      filters.attr ||
      (filters.status && filters.status !== 'all') ||
      filters.minDuration !== undefined ||
      filters.maxDuration !== undefined ||
      filters.timeRangeMinutes
  )
}

export function mergeUniqueTraces(...groups: TraceSummaryDTO[][]): TraceSummaryDTO[] {
  const merged: TraceSummaryDTO[] = []
  const seen = new Set<string>()

  for (const group of groups) {
    for (const trace of group) {
      if (seen.has(trace.traceId)) continue
      merged.push(trace)
      seen.add(trace.traceId)
    }
  }

  return merged
}
