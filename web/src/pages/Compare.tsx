import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { GitCompareArrows, Layers3, ScanSearch, TimerReset } from 'lucide-react'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { PageState } from '@/components/ui/page-state'
import { api } from '@/api/client'
import { getErrorMessage } from '@/lib/errors'
import type { TraceDetailDTO, TraceComparisonDTO } from '@/types'

function findRootSpan(trace: TraceDetailDTO): string {
  return trace.spans.find((span) => !span.parentSpanId)?.name ?? trace.spans[0]?.name ?? 'Unknown root'
}

export function ComparePage() {
  const [searchParams] = useSearchParams()
  const baseId = searchParams.get('base') ?? ''
  const compareId = searchParams.get('compare') ?? ''
  const [base, setBase] = useState<TraceDetailDTO | null>(null)
  const [compare, setCompare] = useState<TraceDetailDTO | null>(null)
  const [diff, setDiff] = useState<TraceComparisonDTO | null>(null)
  const [loading, setLoading] = useState(Boolean(baseId && compareId))
  const [error, setError] = useState<string | null>(null)
  const requestIdRef = useRef(0)
  const abortRef = useRef<AbortController | null>(null)

  const loadComparison = useCallback(async (nextBaseId: string, nextCompareId: string) => {
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    const requestId = ++requestIdRef.current

    setLoading(true)
    setError(null)
    setBase(null)
    setCompare(null)
    setDiff(null)

    try {
      const [nextBase, nextCompare, nextDiff] = await Promise.all([
        api.getTrace(nextBaseId, { signal: controller.signal }),
        api.getTrace(nextCompareId, { signal: controller.signal }),
        api.compareTraces(nextBaseId, nextCompareId, { signal: controller.signal }),
      ])
      if (requestId !== requestIdRef.current) return
      setBase(nextBase)
      setCompare(nextCompare)
      setDiff(nextDiff)
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        return
      }
      if (requestId !== requestIdRef.current) return
      setError(getErrorMessage(err, 'Failed to load trace comparison.'))
    } finally {
      if (requestId === requestIdRef.current) {
        setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    if (!baseId || !compareId) return
    const timer = window.setTimeout(() => {
      void loadComparison(baseId, compareId)
    }, 0)

    return () => {
      window.clearTimeout(timer)
      abortRef.current?.abort()
    }
  }, [baseId, compareId, loadComparison])

  const comparisonSummary = useMemo(() => {
    if (!base || !compare || !diff) return null
    return [
      {
        label: 'Duration delta',
        value: `${diff.durationDeltaMs > 0 ? '+' : ''}${diff.durationDeltaMs.toFixed(1)}ms`,
        tone: diff.durationDeltaMs > 0 ? 'text-red-600 dark:text-red-300' : 'text-emerald-600 dark:text-emerald-300',
        note: diff.durationDeltaMs > 0 ? 'Compare trace is slower' : 'Compare trace is faster',
      },
      {
        label: 'Span delta',
        value: `${diff.spanCountDelta > 0 ? '+' : ''}${diff.spanCountDelta}`,
        tone: 'text-foreground',
        note: `${diff.onlyInBase.length} base-only, ${diff.onlyInCompare.length} compare-only spans`,
      },
      {
        label: 'Error delta',
        value: `${diff.errorDelta > 0 ? '+' : ''}${diff.errorDelta}`,
        tone: diff.errorDelta > 0 ? 'text-red-600 dark:text-red-300' : 'text-foreground',
        note: diff.errorDelta === 0 ? 'Error count is unchanged' : 'Difference in failing spans between traces',
      },
    ]
  }, [base, compare, diff])

  if (!baseId || !compareId) {
    return (
      <PageState
        title="Select two traces to compare"
        description="Open this page with both ?base=TRACE_ID and ?compare=TRACE_ID in the URL."
      />
    )
  }

  if (loading && (!base || !compare)) {
    return <PageState title="Loading trace comparison" description="Fetching both traces and their structural diff." />
  }

  if (error) {
    return <PageState title="Unable to compare traces" description={error} actionLabel="Retry" onAction={() => void loadComparison(baseId, compareId)} />
  }

  if (!base || !compare || !diff) {
    return <PageState title="Comparison unavailable" description="The requested traces could not be resolved." />
  }

  const maxDuration = Math.max(base.durationMs, compare.durationMs)
  const grayedBaseIds = new Set(diff.onlyInBase)
  const highlightedCompareIds = new Set(diff.onlyInCompare)
  const baseDeltaMap = new Map(diff.matched.map((match) => [match.baseSpanId, match.durationDeltaMs]))

  return (
    <div className="mx-auto max-w-7xl space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-50px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%),radial-gradient(circle_at_bottom_right,_rgba(52,211,153,0.12),_transparent_38%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.95fr)]">
          <div className="space-y-4">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <GitCompareArrows className="h-3.5 w-3.5" />
              Structural diff
            </div>
            <h1 className="max-w-2xl text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
              Compare two executions on a shared time axis before you chase the wrong regression.
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
              Overlay both traces against the same duration envelope, then inspect span count drift, structural
              divergence, and latency inflation without mentally normalizing two separate views.
            </p>
            <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
              <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1.5">Base: {baseId}</span>
              <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1.5">Compare: {compareId}</span>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            {comparisonSummary?.map((item) => (
              <div key={item.label} className="rounded-[24px] border border-border/70 bg-background/70 p-4">
                <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">{item.label}</div>
                <div className={`mt-3 text-3xl font-semibold ${item.tone}`}>{item.value}</div>
                <div className="mt-1 text-xs text-muted-foreground">{item.note}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        {[
          {
            key: 'base',
            title: 'Base trace',
            trace: base,
            accent: 'Only in base',
            subtitle: `Root span: ${findRootSpan(base)}`,
            extra: `${base.durationMs.toFixed(1)}ms · ${base.spanCount} spans · ${base.errorCount} errors`,
            grayedSpanIds: grayedBaseIds,
            durationDeltas: baseDeltaMap,
            highlightedSpanIds: undefined,
          },
          {
            key: 'compare',
            title: 'Compare trace',
            trace: compare,
            accent: 'Only in compare',
            subtitle: `Root span: ${findRootSpan(compare)}`,
            extra: `${compare.durationMs.toFixed(1)}ms · ${compare.spanCount} spans · ${compare.errorCount} errors`,
            grayedSpanIds: undefined,
            durationDeltas: undefined,
            highlightedSpanIds: highlightedCompareIds,
          },
        ].map((panel) => (
          <section key={panel.key} className="overflow-hidden rounded-[28px] border border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
            <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border/70 px-5 py-4">
              <div>
                <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">{panel.title}</div>
                <div className="mt-1 text-lg font-semibold text-foreground">{panel.subtitle}</div>
                <div className="mt-1 text-sm text-muted-foreground">{panel.extra}</div>
              </div>
              <div className="flex flex-wrap gap-2 text-xs">
                <span className="rounded-full border border-border/70 bg-background/70 px-2.5 py-1">{panel.accent}</span>
                <span className="rounded-full border border-border/70 bg-background/70 px-2.5 py-1">{panel.trace.services.length} services</span>
              </div>
            </div>
            <div className="p-4">
              <WaterfallChart
                trace={{ ...panel.trace, durationMs: maxDuration }}
                onSpanSelect={() => undefined}
                criticalPathIds={new Set()}
                grayedSpanIds={panel.grayedSpanIds}
                highlightedSpanIds={panel.highlightedSpanIds}
                durationDeltas={panel.durationDeltas}
              />
            </div>
          </section>
        ))}
      </section>

      <section className="rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
        <div className="grid gap-3 md:grid-cols-3">
          <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
            <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              <TimerReset className="h-3.5 w-3.5" />
              Shared scale
            </div>
            <div className="mt-2 text-sm text-muted-foreground">Both traces use the same duration envelope so visual drift is comparable at a glance.</div>
          </div>
          <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
            <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              <Layers3 className="h-3.5 w-3.5" />
              Base-only spans
            </div>
            <div className="mt-2 text-sm text-muted-foreground">{diff.onlyInBase.length} spans are unique to the base execution and rendered dimmed on the left.</div>
          </div>
          <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
            <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              <ScanSearch className="h-3.5 w-3.5" />
              Compare-only spans
            </div>
            <div className="mt-2 text-sm text-muted-foreground">{diff.onlyInCompare.length} spans are unique to the compare execution and highlighted on the right.</div>
          </div>
        </div>
      </section>
    </div>
  )
}
