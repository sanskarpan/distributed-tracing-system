import { useState, useEffect, useRef } from 'react'
import { api } from '@/api/client'
import type { TraceListResponse, TraceSummaryDTO } from '@/types'

export interface SearchFilters {
  service?: string
  operation?: string
  attr?: string
  status?: 'all' | 'error'
  minDuration?: number
  maxDuration?: number
  timeRangeMinutes?: number
  limit?: number
  offset?: number
  sortBy?: string
  sortDesc?: boolean
}

function buildTraceQueryParams(filters: SearchFilters): Record<string, string | number | boolean | undefined> {
  const params: Record<string, string | number | boolean | undefined> = {}
  if (filters.service) params.service = filters.service
  if (filters.operation) params.operation = filters.operation
  if (filters.attr) params.attr = filters.attr
  if (filters.status && filters.status !== 'all') params.status = filters.status
  if (filters.minDuration !== undefined) params.minDuration = filters.minDuration
  if (filters.maxDuration !== undefined) params.maxDuration = filters.maxDuration
  if (filters.timeRangeMinutes) {
    params.startTime = Date.now() - filters.timeRangeMinutes * 60 * 1000
  }
  if (filters.limit !== undefined) params.limit = filters.limit
  if (filters.offset !== undefined) params.offset = filters.offset
  if (filters.sortBy) params.sortBy = filters.sortBy
  if (filters.sortDesc !== undefined) params.sortDesc = filters.sortDesc
  return params
}

function mergeTracePages(existing: TraceSummaryDTO[], incoming: TraceSummaryDTO[]): TraceSummaryDTO[] {
  const merged = [...existing]
  const seen = new Set(existing.map((trace) => trace.traceId))

  for (const trace of incoming) {
    if (seen.has(trace.traceId)) continue
    merged.push(trace)
    seen.add(trace.traceId)
  }

  return merged
}

export function useSearch(initialFilters: SearchFilters) {
  const [filters, setFilters] = useState<SearchFilters>(initialFilters)
  const [results, setResults] = useState<TraceListResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const requestIdRef = useRef(0)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current)
    }

    timerRef.current = setTimeout(() => {
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      const requestId = ++requestIdRef.current
      const params = buildTraceQueryParams(filters)

      setLoading(true)
      api
        .getTraces(params, { signal: controller.signal })
        .then((r) => {
          if (requestId !== requestIdRef.current) return
          setResults((prev) => {
            if ((filters.offset ?? 0) <= 0 || prev === null) {
              return r
            }
            return {
              ...r,
              traces: mergeTracePages(prev.traces, r.traces),
            }
          })
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === 'AbortError') {
            return
          }
          console.error(err)
        })
        .finally(() => {
          if (requestId === requestIdRef.current) {
            setLoading(false)
          }
        })
    }, 300)

    return () => {
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current)
      }
      abortRef.current?.abort()
    }
  }, [filters])

  return { results, loading, filters, setFilters }
}
