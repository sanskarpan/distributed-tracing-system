import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { PageState } from '@/components/ui/page-state'
import { api } from '@/api/client'
import { getErrorMessage } from '@/lib/errors'
import type { TraceDetailDTO, TraceComparisonDTO } from '@/types'

export function ComparePage() {
  const [searchParams] = useSearchParams()
  const baseId = searchParams.get('base') ?? ''
  const compareId = searchParams.get('compare') ?? ''
  const [base, setBase] = useState<TraceDetailDTO | null>(null)
  const [compare, setCompare] = useState<TraceDetailDTO | null>(null)
  const [diff, setDiff] = useState<TraceComparisonDTO | null>(null)
  const [loading, setLoading] = useState(Boolean(baseId && compareId))
  const [error, setError] = useState<string | null>(null)

  const loadComparison = useCallback(async (nextBaseId: string, nextCompareId: string) => {
    setLoading(true)
    setError(null)
    setBase(null)
    setCompare(null)
    setDiff(null)

    try {
      const [nextBase, nextCompare, nextDiff] = await Promise.all([
        api.getTrace(nextBaseId),
        api.getTrace(nextCompareId),
        api.compareTraces(nextBaseId, nextCompareId),
      ])
      setBase(nextBase)
      setCompare(nextCompare)
      setDiff(nextDiff)
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Failed to load trace comparison.'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!baseId || !compareId) return
    const timer = window.setTimeout(() => {
      void loadComparison(baseId, compareId)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [baseId, compareId, loadComparison])

  if (!baseId || !compareId) {
    return (
      <PageState
        title="Select two traces to compare"
        description="Open this page with both ?base=TRACE_ID and ?compare=TRACE_ID in the URL."
      />
    )
  }

  if (loading && (!base || !compare)) {
    return <PageState title="Loading trace comparison" description="Fetching both traces and their diff." />
  }

  if (error) {
    return <PageState title="Unable to compare traces" description={error} actionLabel="Retry" onAction={() => void loadComparison(baseId, compareId)} />
  }

  if (!base || !compare || !diff) {
    return <PageState title="Comparison unavailable" description="The requested traces could not be resolved." />
  }

  const maxDuration = Math.max(base.durationMs, compare.durationMs)

  const grayedBaseIds = diff ? new Set(diff.onlyInBase) : undefined
  const highlightedCompareIds = diff ? new Set(diff.onlyInCompare) : undefined
  const baseDeltaMap = diff
    ? new Map(diff.matched.map(m => [m.baseSpanId, m.durationDeltaMs]))
    : undefined

  return (
    <div className="p-4 space-y-4">
      <div className="flex items-center gap-4 flex-wrap">
        <h1 className="text-xl font-bold">Compare Traces</h1>
        {diff && (
          <div className="flex gap-3 text-sm">
            <span className={diff.durationDeltaMs > 0 ? 'text-red-600' : 'text-green-600'}>
              &Delta; {diff.durationDeltaMs > 0 ? '+' : ''}{diff.durationDeltaMs.toFixed(1)}ms
            </span>
            <span>
              &Delta; spans: {diff.spanCountDelta > 0 ? '+' : ''}{diff.spanCountDelta}
            </span>
            {diff.errorDelta !== 0 && (
              <span className="text-red-600">&Delta; errors: {diff.errorDelta}</span>
            )}
          </div>
        )}
        {diff && (
          <div className="flex gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-3 rounded bg-slate-400 opacity-40" />
              only in base
            </span>
            <span className="flex items-center gap-1">
              <span className="inline-block w-3 h-3 rounded bg-green-500" />
              only in compare
            </span>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <div>
          <div className="text-sm font-medium mb-2 text-muted-foreground">
            Base &middot; {base.durationMs.toFixed(1)}ms
          </div>
          <WaterfallChart
            trace={{ ...base, durationMs: maxDuration }}
            onSpanSelect={() => undefined}
            criticalPathIds={new Set()}
            grayedSpanIds={grayedBaseIds}
            durationDeltas={baseDeltaMap}
          />
        </div>
        <div>
          <div className="text-sm font-medium mb-2 text-muted-foreground">
            Compare &middot; {compare.durationMs.toFixed(1)}ms
          </div>
          <WaterfallChart
            trace={{ ...compare, durationMs: maxDuration }}
            onSpanSelect={() => undefined}
            criticalPathIds={new Set()}
            highlightedSpanIds={highlightedCompareIds}
          />
        </div>
      </div>
    </div>
  )
}
