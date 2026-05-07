import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { api } from '@/api/client'
import type { TraceDetailDTO, TraceComparisonDTO } from '@/types'

export function ComparePage() {
  const [searchParams] = useSearchParams()
  const baseId = searchParams.get('base') ?? ''
  const compareId = searchParams.get('compare') ?? ''
  const [base, setBase] = useState<TraceDetailDTO | null>(null)
  const [compare, setCompare] = useState<TraceDetailDTO | null>(null)
  const [diff, setDiff] = useState<TraceComparisonDTO | null>(null)

  useEffect(() => {
    if (!baseId || !compareId) return
    Promise.all([
      api.getTrace(baseId).then(setBase),
      api.getTrace(compareId).then(setCompare),
      api.compareTraces(baseId, compareId).then(setDiff),
    ]).catch(console.error)
  }, [baseId, compareId])

  if (!base || !compare) {
    return (
      <div className="p-8 text-center text-muted-foreground">
        Select two traces to compare. Use ?base=ID&amp;compare=ID in the URL.
      </div>
    )
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

      <div className="grid grid-cols-2 gap-4">
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
