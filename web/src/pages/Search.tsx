import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { useSearch } from '@/hooks/useSearch'
import { useSSE } from '@/hooks/useSSE'
import { useTracingStore } from '@/store/tracingStore'
import { TraceCard } from '@/components/search/TraceCard'
import { FilterBar } from '@/components/search/FilterBar'
import { Button } from '@/components/ui/button'
import { PageState } from '@/components/ui/page-state'
import { api } from '@/api/client'
import { getErrorMessage } from '@/lib/errors'
import type { TraceSummaryDTO } from '@/types'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'
import { hasActiveSearchFilters, mergeUniqueTraces } from './search-helpers'

const PAGE_SIZE = 20

export function SearchPage() {
  const navigate = useNavigate()
  const { results, loading, error, filters, setFilters } = useSearch({ limit: PAGE_SIZE })
  const { services, setServices, liveTraces, addLiveTrace } = useTracingStore()
  const [operations, setOperations] = useState<string[]>([])
  const [servicesError, setServicesError] = useState<string | null>(null)
  const [operationsError, setOperationsError] = useState<string | null>(null)
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
    api.getServices()
      .then(r => {
        setServices(r.services)
        setServicesError(null)
      })
      .catch((err: unknown) => {
        setServicesError(getErrorMessage(err, 'Failed to load services.'))
      })
  }, [setServices])

  // Fetch operations when service changes
  useEffect(() => {
    if (filters.service) {
      api.getOperations(filters.service)
        .then(r => {
          setOperations(r.operations)
          setOperationsError(null)
        })
        .catch((err: unknown) => {
          setOperations([])
          setOperationsError(getErrorMessage(err, 'Failed to load operations.'))
        })
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
  const operationsForFilter = filters.service ? operations : []
  const operationsWarning = filters.service ? operationsError : null
  const currentOffset = filters.offset ?? 0
  const includeLiveTraces = !hasActiveSearchFilters(filters) && currentOffset === 0
  const allTraces = mergeUniqueTraces(
    includeLiveTraces ? liveTraces : [],
    queryTraces
  ).slice(0, 500)
  const maxDuration = Math.max(...allTraces.map(t => t.durationMs), 1)
  const hasMore = results?.hasMore ?? false
  const isFiltering = hasActiveSearchFilters(filters)

  const loadMore = () => {
    setFilters({ ...filters, offset: currentOffset + PAGE_SIZE })
  }

  return (
    <div className="p-4 max-w-4xl mx-auto">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-bold">Traces</h1>
        {loading && <span className="text-sm text-muted-foreground">Loading&hellip;</span>}
      </div>

      {(servicesError || operationsWarning) && (
        <div className="mb-3 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          {servicesError ?? operationsWarning}
        </div>
      )}

      <div ref={serviceSelectRef}>
        <FilterBar
          services={services}
          operations={operationsForFilter}
          filters={filters}
          onChange={(f) => setFilters({ ...f, offset: 0 })}
        />
      </div>
      <div className="mt-1 text-xs text-muted-foreground flex gap-4">
        <span><kbd className="font-mono bg-muted px-1 rounded">/</kbd> filter by service</span>
        <span><kbd className="font-mono bg-muted px-1 rounded">r</kbd> reset filters</span>
      </div>

      <div className="mt-4 space-y-2">
        {error ? (
          <PageState
            title="Unable to load traces"
            description={error}
            actionLabel="Retry"
            onAction={() => setFilters({ ...filters })}
          />
        ) : (
          <AnimatePresence mode="popLayout">
            {allTraces.length === 0 && !loading && (
              <motion.div
                key="empty"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
              >
                <PageState
                  title={isFiltering ? 'No traces match these filters' : 'No traces yet'}
                  description={
                    isFiltering
                      ? 'Try broadening the service, operation, attribute, duration, or time filters.'
                      : 'Ingest spans or run the demo traffic generator to populate the tracing view.'
                  }
                  actionLabel={isFiltering ? 'Reset filters' : undefined}
                  onAction={isFiltering ? () => setFilters({ limit: PAGE_SIZE }) : undefined}
                />
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
        )}
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
