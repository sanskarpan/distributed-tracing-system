import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Activity, Clock3, Layers3, SearchCode } from 'lucide-react'
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
    const controller = new AbortController()

    api.getServices({ signal: controller.signal })
      .then(r => {
        setServices(r.services)
        setServicesError(null)
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === 'AbortError') {
          return
        }
        setServicesError(getErrorMessage(err, 'Failed to load services.'))
      })

    return () => controller.abort()
  }, [setServices])

  useEffect(() => {
    if (filters.service) {
      const controller = new AbortController()

      api.getOperations(filters.service, { signal: controller.signal })
        .then(r => {
          setOperations(r.operations)
          setOperationsError(null)
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === 'AbortError') {
            return
          }
          setOperations([])
          setOperationsError(getErrorMessage(err, 'Failed to load operations.'))
        })

      return () => controller.abort()
    }
  }, [filters.service])

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
  const errorTraceCount = allTraces.filter((trace) => trace.hasError).length
  const visibleServiceCount = new Set(allTraces.flatMap((trace) => trace.services)).size
  const maxDuration = Math.max(...allTraces.map(t => t.durationMs), 1)
  const hasMore = results?.hasMore ?? false
  const isFiltering = hasActiveSearchFilters(filters)

  const loadMore = () => {
    setFilters({ ...filters, offset: currentOffset + PAGE_SIZE })
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-48px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.16),_transparent_42%),radial-gradient(circle_at_bottom_right,_rgba(244,114,182,0.12),_transparent_38%)]" />
        <div className="relative grid gap-6 lg:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]">
          <div className="space-y-4">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">
              <SearchCode className="h-3.5 w-3.5" />
              Live search surface
            </div>
            <div className="space-y-3">
              <h1 className="max-w-2xl text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
                Find the traces worth opening before the incident window moves on.
              </h1>
              <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
                Blend live arrivals with queryable history, then narrow by service, operation, latency, attributes,
                and failure mode without leaving the investigation surface.
              </p>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Layers3 className="h-3.5 w-3.5" />
                Visible traces
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{allTraces.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {results?.total ?? allTraces.length} total returned in the current query window
              </div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Activity className="h-3.5 w-3.5" />
                Erroring traces
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{errorTraceCount}</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {visibleServiceCount} services represented in the visible result set
              </div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Clock3 className="h-3.5 w-3.5" />
                Operator shortcuts
              </div>
              <div className="mt-3 flex flex-wrap gap-2 text-xs">
                <span className="rounded-full border border-border/70 bg-card px-2.5 py-1"><kbd className="font-mono">/</kbd> focus service</span>
                <span className="rounded-full border border-border/70 bg-card px-2.5 py-1"><kbd className="font-mono">r</kbd> reset query</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-50px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">Query controls</div>
            <div className="mt-1 text-sm text-muted-foreground">Tune scope, error state, and latency without leaving the trace list.</div>
          </div>
          {loading && <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1 text-xs text-muted-foreground">Refreshing…</span>}
        </div>

        {(servicesError || operationsWarning) && (
          <div className="mb-4 rounded-2xl border border-amber-300 bg-amber-50/95 px-4 py-3 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
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

        <div className="mt-4 flex flex-wrap gap-3 text-xs text-muted-foreground">
          <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1"><kbd className="font-mono">/</kbd> focus service</span>
          <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1"><kbd className="font-mono">r</kbd> reset filters</span>
          <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1">
            {isFiltering ? 'Filtered investigation mode' : 'Live stream blended with the first page of results'}
          </span>
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">Trace queue</div>
            <h2 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
              {isFiltering ? 'Filtered candidates' : 'Recent arrivals and query matches'}
            </h2>
          </div>
          <div className="rounded-full border border-border/70 bg-card/80 px-3 py-1.5 text-sm text-muted-foreground">
            Showing {allTraces.length} trace{allTraces.length !== 1 ? 's' : ''}
          </div>
        </div>

        <div className="space-y-3">
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
          <div className="flex justify-center pt-2">
            <Button variant="outline" onClick={loadMore} disabled={loading}>
              Load more
            </Button>
          </div>
        )}
      </section>
    </div>
  )
}
