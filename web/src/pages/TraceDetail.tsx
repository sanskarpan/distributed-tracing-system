import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { SpanDrawer } from '@/components/waterfall/SpanDrawer'
import { api } from '@/api/client'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'
import { Badge } from '@/components/ui/badge'

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [trace, setTrace] = useState<TraceDetailDTO | null>(null)
  const [selectedSpan, setSelectedSpan] = useState<SpanDetailDTO | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)

  useEffect(() => {
    if (!id) return
    api.getTrace(id).then(setTrace).catch(console.error)
  }, [id])

  if (!trace) {
    return (
      <div className="p-8 text-center text-muted-foreground">Loading trace&hellip;</div>
    )
  }

  const criticalPathIds = new Set(trace.criticalPath)

  return (
    <div className="p-4">
      <div className="mb-4 flex items-center gap-3 flex-wrap">
        <h1 className="text-lg font-bold font-mono">{id?.slice(0, 16)}&hellip;</h1>
        <Badge variant="outline">{trace.spanCount} spans</Badge>
        <Badge variant="outline">{trace.durationMs.toFixed(1)}ms</Badge>
        {trace.errorCount > 0 && (
          <Badge variant="destructive">{trace.errorCount} errors</Badge>
        )}
        <div className="flex gap-1 flex-wrap">
          {trace.services.map(s => (
            <Badge key={s} variant="secondary" className="text-xs">
              {s}
            </Badge>
          ))}
        </div>
      </div>

      <WaterfallChart
        trace={trace}
        onSpanSelect={(span) => {
          setSelectedSpan(span)
          setDrawerOpen(true)
        }}
        criticalPathIds={criticalPathIds}
      />

      <SpanDrawer
        span={selectedSpan}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        allSpans={trace.spans}
        onParentClick={(parentSpanId) => {
          const parent = trace.spans.find(s => s.spanId === parentSpanId)
          if (parent) setSelectedSpan(parent)
        }}
      />
    </div>
  )
}
