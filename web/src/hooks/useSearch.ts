import { useState, useEffect, useRef } from 'react'
import { api } from '@/api/client'
import type { TraceListResponse } from '@/types'

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

export function useSearch(initialFilters: SearchFilters) {
  const [filters, setFilters] = useState<SearchFilters>(initialFilters)
  const [results, setResults] = useState<TraceListResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current)
    }

    timerRef.current = setTimeout(() => {
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

      setLoading(true)
      api
        .getTraces(params)
        .then((r) => {
          setResults(r)
        })
        .catch(console.error)
        .finally(() => {
          setLoading(false)
        })
    }, 300)

    return () => {
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current)
      }
    }
  }, [filters])

  return { results, loading, filters, setFilters }
}
