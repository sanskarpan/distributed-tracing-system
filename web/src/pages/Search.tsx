import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { useSearch } from '@/hooks/useSearch'
import { useSSE } from '@/hooks/useSSE'
import { useTracingStore } from '@/store/tracingStore'
import { TraceCard } from '@/components/search/TraceCard'
import { FilterBar } from '@/components/search/FilterBar'
import { Button } from '@/components/ui/button'
import { api } from '@/api/client'
import type { TraceSummaryDTO } from '@/types'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'

const PAGE_SIZE = 20

export function SearchPage() {
  const navigate = useNavigate()
  const { results, loading, filters, setFilters } = useSearch({ limit: PAGE_SIZE })
  const { services, setServices, liveTraces, addLiveTrace } = useTracingStore()
  const [operations, setOperations] = useState<string[]>([])
  const serviceSelectRef = useRef<HTMLDivElement>(null)

  useKeyboardShortcuts([
    {
      key: '/',
      description: 'Focus service filter',
      handler: () => {
        const input = serviceSelectRef.current?.querySelector('button')
        input?.click()
      },
    },
    {
      key: 'r',
      description: 'Reset filters',
      handler: () => setFilters({ limit: PAGE_SIZE }),
    },
  ])

  useEffect(() => {
    api.getServices().then(r => setServices(r.services)).catch(console.error)
  }, [setServices])

  // Fetch operations when service changes
  useEffect(() => {
    if (filters.service) {
      api.getOperations(filters.service)
        .then(r => setOperations(r.operations))
        .catch(console.error)
    } else {
      setOperations([])
    }
  }, [filters.service])

  // Live SSE updates
  useSSE('/sse/traces', (event: unknown) => {
    const e = event as { type: string; data: TraceSummaryDTO }
    if (e.type === 'trace' && e.data) {
      addLiveTrace(e.data)
    }
  })

  const queryTraces = results?.traces ?? []
  const allTraces = [...liveTraces, ...queryTraces].slice(0, 500)
  const maxDuration = Math.max(...allTraces.map(t => t.durationMs), 1)
  const hasMore = results?.hasMore ?? false
  const currentOffset = filters.offset ?? 0

  const loadMore = () => {
    setFilters({ ...filters, offset: currentOffset + PAGE_SIZE })
  }

  return (
    <div className="p-4 max-w-4xl mx-auto">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-bold">Traces</h1>
        {loading && <span className="text-sm text-muted-foreground">Loading&hellip;</span>}
      </div>

      <div ref={serviceSelectRef}>
        <FilterBar
          services={services}
          operations={operations}
          filters={filters}
          onChange={(f) => setFilters({ ...f, offset: 0 })}
        />
      </div>
      <div className="mt-1 text-xs text-muted-foreground flex gap-4">
        <span><kbd className="font-mono bg-muted px-1 rounded">/</kbd> filter by service</span>
        <span><kbd className="font-mono bg-muted px-1 rounded">r</kbd> reset filters</span>
      </div>

      <div className="mt-4 space-y-2">
        <AnimatePresence mode="popLayout">
          {allTraces.length === 0 && !loading && (
            <motion.div
              key="empty"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              className="text-center py-12 text-muted-foreground"
            >
              No traces found. Ingest spans to see traces here.
            </motion.div>
          )}
          {allTraces.map((trace) => (
            <motion.div
              key={trace.traceId}
              initial={{ opacity: 0, y: -10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
            >
              <TraceCard
                trace={trace}
                maxDuration={maxDuration}
                onClick={() => navigate(`/trace/${trace.traceId}`)}
              />
            </motion.div>
          ))}
        </AnimatePresence>
      </div>

      {hasMore && (
        <div className="mt-4 flex justify-center">
          <Button variant="outline" onClick={loadMore} disabled={loading}>
            Load more
          </Button>
        </div>
      )}
    </div>
  )
}
