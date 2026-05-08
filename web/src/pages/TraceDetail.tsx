import { useEffect, useState, useCallback, useRef, useMemo } from 'react'
import { useParams } from 'react-router-dom'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { FlameGraph } from '@/components/waterfall/FlameGraph'
import { SpanDrawer } from '@/components/waterfall/SpanDrawer'
import { api } from '@/api/client'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [trace, setTrace] = useState<TraceDetailDTO | null>(null)
  const [selectedSpan, setSelectedSpan] = useState<SpanDetailDTO | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const [spanFilter, setSpanFilter] = useState('')
  const [viewMode, setViewMode] = useState<'waterfall' | 'flame'>('waterfall')

  const spanIndexRef = useRef(0)

  const exportChart = useCallback((format: 'svg' | 'png') => {
    const svgEl = document.querySelector<SVGSVGElement>('.waterfall-svg, .flame-svg')
    if (!svgEl) return
    const serializer = new XMLSerializer()
    const svgStr = serializer.serializeToString(svgEl)
    if (format === 'svg') {
      const blob = new Blob([svgStr], { type: 'image/svg+xml' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = `trace-${id}.svg`; a.click()
      URL.revokeObjectURL(url)
    } else {
      const canvas = document.createElement('canvas')
      canvas.width = svgEl.clientWidth || 1200
      canvas.height = svgEl.clientHeight || 600
      const ctx = canvas.getContext('2d')!
      const img = new Image()
      img.onload = () => {
        ctx.fillStyle = 'white'
        ctx.fillRect(0, 0, canvas.width, canvas.height)
        ctx.drawImage(img, 0, 0)
        const a = document.createElement('a')
        a.href = canvas.toDataURL('image/png')
        a.download = `trace-${id}.png`; a.click()
      }
      img.src = 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(svgStr)))
    }
  }, [id])

  useKeyboardShortcuts([
    {
      key: 'Escape',
      description: 'Close span drawer',
      handler: () => setDrawerOpen(false),
    },
    {
      key: 'j',
      description: 'Select next span',
      handler: () => {
        if (!trace) return
        spanIndexRef.current = Math.min(spanIndexRef.current + 1, trace.spans.length - 1)
        setSelectedSpan(trace.spans[spanIndexRef.current])
        setDrawerOpen(true)
      },
    },
    {
      key: 'k',
      description: 'Select previous span',
      handler: () => {
        if (!trace) return
        spanIndexRef.current = Math.max(spanIndexRef.current - 1, 0)
        setSelectedSpan(trace.spans[spanIndexRef.current])
        setDrawerOpen(true)
      },
    },
    {
      key: 'e',
      description: 'Export trace as JSON',
      handler: () => {
        if (id) window.location.href = `/api/v1/traces/${id}/export`
      },
    },
  ])

  const copyLink = useCallback(() => {
    navigator.clipboard.writeText(window.location.href).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }, [])

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

  // Span attribute filter: dim non-matching spans
  const { grayedSpanIds, highlightedSpanIds } = (() => {
    const trimmed = spanFilter.trim()
    if (!trimmed) return { grayedSpanIds: undefined, highlightedSpanIds: undefined }
    const [filterKey, filterVal] = trimmed.includes('=') ? trimmed.split('=', 2) : [trimmed, '']
    const key = filterKey.toLowerCase()
    const val = filterVal.toLowerCase()
    const matched = new Set<string>()
    const grayed = new Set<string>()
    for (const span of trace.spans) {
      const hits = span.attributes.some(attr => {
        if (!attr.key.toLowerCase().includes(key)) return false
        if (!val) return true
        const sv = (attr.stringValue ?? '').toLowerCase()
        return sv.includes(val)
      })
      if (hits) matched.add(span.spanId)
      else grayed.add(span.spanId)
    }
    return { grayedSpanIds: grayed, highlightedSpanIds: matched }
  })()

  // Root cause: the earliest span with an error status (code 2 = StatusError in OTLP)
  const rootCauseSpan = useMemo(() =>
    trace.errorCount > 0
      ? trace.spans
          .filter(s => s.status.code === 2)
          .sort((a, b) => Number(a.startTimeUnixNano) - Number(b.startTimeUnixNano))[0] ?? null
      : null,
    [trace]
  )

  return (
    <div className="p-4">
      <div className="mb-4 flex items-center gap-3 flex-wrap">
        <h1 className="text-lg font-bold font-mono">{id?.slice(0, 16)}&hellip;</h1>
        <Badge variant="outline">{trace.spanCount} spans</Badge>
        <Badge variant="outline">{trace.durationMs.toFixed(1)}ms</Badge>
        {trace.errorCount > 0 && (
          <Badge variant="destructive">{trace.errorCount} errors</Badge>
        )}
        {rootCauseSpan && (
          <button
            className="text-xs px-2 py-1 rounded bg-red-100 text-red-700 border border-red-300 hover:bg-red-200 transition-colors"
            onClick={() => { setSelectedSpan(rootCauseSpan); setDrawerOpen(true) }}
          >
            Root cause: {rootCauseSpan.name}
          </button>
        )}
        <div className="flex gap-1 flex-wrap">
          {trace.services.map(s => (
            <Badge key={s} variant="secondary" className="text-xs">
              {s}
            </Badge>
          ))}
        </div>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" size="sm" onClick={copyLink}>
            {copied ? 'Copied!' : 'Copy link'}
          </Button>
          <Button variant="outline" size="sm" onClick={() => exportChart('svg')}>Export SVG</Button>
          <Button variant="outline" size="sm" onClick={() => exportChart('png')}>Export PNG</Button>
          <a
            href={`/api/v1/traces/${id}/export`}
            download={`trace-${id}.json`}
          >
            <Button variant="outline" size="sm">Export JSON</Button>
          </a>
        </div>
      </div>

      <div className="mb-3 flex items-center gap-3">
        <input
          type="text"
          placeholder="Filter spans: key=value or key"
          value={spanFilter}
          onChange={(e) => setSpanFilter(e.target.value)}
          className="h-8 w-64 rounded-md border border-input bg-background px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
        />
        {spanFilter && highlightedSpanIds && (
          <span className="text-xs text-muted-foreground">
            {highlightedSpanIds.size} match{highlightedSpanIds.size !== 1 ? 'es' : ''}
          </span>
        )}
        <div className="ml-auto flex rounded-md border overflow-hidden text-xs">
          <button
            className={`px-3 py-1 ${viewMode === 'waterfall' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
            onClick={() => setViewMode('waterfall')}
          >
            Waterfall
          </button>
          <button
            className={`px-3 py-1 ${viewMode === 'flame' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'}`}
            onClick={() => setViewMode('flame')}
          >
            Flame
          </button>
        </div>
      </div>

      {viewMode === 'waterfall' ? (
        <WaterfallChart
          trace={trace}
          onSpanSelect={(span) => {
            setSelectedSpan(span)
            setDrawerOpen(true)
          }}
          criticalPathIds={criticalPathIds}
          grayedSpanIds={grayedSpanIds}
          highlightedSpanIds={highlightedSpanIds}
        />
      ) : (
        <FlameGraph
          trace={trace}
          onSpanSelect={(span) => {
            setSelectedSpan(span)
            setDrawerOpen(true)
          }}
          criticalPathIds={criticalPathIds}
          grayedSpanIds={grayedSpanIds}
          highlightedSpanIds={highlightedSpanIds}
        />
      )}

      <div className="mt-2 text-xs text-muted-foreground flex gap-4">
        <span><kbd className="font-mono bg-muted px-1 rounded">j</kbd>/<kbd className="font-mono bg-muted px-1 rounded">k</kbd> navigate spans</span>
        <span><kbd className="font-mono bg-muted px-1 rounded">Esc</kbd> close drawer</span>
        <span><kbd className="font-mono bg-muted px-1 rounded">e</kbd> export JSON</span>
      </div>

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
