import { useEffect, useState, useCallback, useRef } from 'react'
import { useParams } from 'react-router-dom'
import { AlertTriangle, Clock3, Copy, Download, Filter, GitBranchPlus, Layers3 } from 'lucide-react'
import { WaterfallChart } from '@/components/waterfall/WaterfallChart'
import { FlameGraph } from '@/components/waterfall/FlameGraph'
import { SpanDrawer } from '@/components/waterfall/SpanDrawer'
import { api } from '@/api/client'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useKeyboardShortcuts } from '@/hooks/useKeyboardShortcuts'
import { PageState } from '@/components/ui/page-state'
import { getErrorMessage } from '@/lib/errors'

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }

  const input = document.createElement('textarea')
  input.value = value
  input.setAttribute('readonly', 'true')
  input.style.position = 'absolute'
  input.style.left = '-9999px'
  document.body.appendChild(input)
  input.select()

  const copied = document.execCommand('copy')
  document.body.removeChild(input)

  if (!copied) {
    throw new Error('Clipboard access is unavailable in this browser context.')
  }
}

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [trace, setTrace] = useState<TraceDetailDTO | null>(null)
  const [selectedSpan, setSelectedSpan] = useState<SpanDetailDTO | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const [spanFilter, setSpanFilter] = useState('')
  const [viewMode, setViewMode] = useState<'waterfall' | 'flame'>('waterfall')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const spanIndexRef = useRef(0)
  const requestIdRef = useRef(0)
  const abortRef = useRef<AbortController | null>(null)

  const exportChart = useCallback((format: 'svg' | 'png') => {
    const svgEl = document.querySelector<SVGSVGElement>('.waterfall-svg, .flame-svg')
    if (!svgEl) {
      setActionError('The current chart is not ready to export yet.')
      return
    }
    const serializer = new XMLSerializer()
    const svgStr = serializer.serializeToString(svgEl)
    setActionError(null)

    try {
      if (format === 'svg') {
        const blob = new Blob([svgStr], { type: 'image/svg+xml' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `trace-${id}.svg`
        a.click()
        URL.revokeObjectURL(url)
        return
      }

      const canvas = document.createElement('canvas')
      canvas.width = svgEl.clientWidth || 1200
      canvas.height = svgEl.clientHeight || 600
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        throw new Error('Canvas rendering is unavailable for PNG export.')
      }

      const img = new Image()
      img.onerror = () => setActionError('The browser could not render the SVG for PNG export.')
      img.onload = () => {
        ctx.fillStyle = 'white'
        ctx.fillRect(0, 0, canvas.width, canvas.height)
        ctx.drawImage(img, 0, 0)
        const a = document.createElement('a')
        a.href = canvas.toDataURL('image/png')
        a.download = `trace-${id}.png`
        a.click()
      }
      img.src = `data:image/svg+xml;base64,${btoa(unescape(encodeURIComponent(svgStr)))}`
    } catch (err: unknown) {
      setActionError(getErrorMessage(err, 'Unable to export the current chart.'))
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
    setActionError(null)
    copyText(window.location.href)
      .then(() => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 2000)
      })
      .catch((err: unknown) => {
        setActionError(getErrorMessage(err, 'Unable to copy the trace URL.'))
      })
  }, [])

  const reloadTrace = useCallback(async () => {
    if (!id) return

    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    const requestId = ++requestIdRef.current

    setLoading(true)
    setError(null)

    try {
      const nextTrace = await api.getTrace(id, { signal: controller.signal })
      if (requestId !== requestIdRef.current) return
      setTrace(nextTrace)
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        return
      }
      if (requestId !== requestIdRef.current) return
      setError(getErrorMessage(err, 'Failed to load trace detail.'))
    } finally {
      if (requestId === requestIdRef.current) {
        setLoading(false)
      }
    }
  }, [id])

  useEffect(() => {
    if (!id) return
    const timer = window.setTimeout(() => {
      void reloadTrace()
    }, 0)

    return () => {
      window.clearTimeout(timer)
      abortRef.current?.abort()
    }
  }, [id, reloadTrace])

  const rootCauseSpan = trace?.errorCount && trace.errorCount > 0
    ? trace.spans
        .filter((span) => span.status.code === 2)
        .sort((left, right) => Number(left.startTimeUnixNano) - Number(right.startTimeUnixNano))[0] ?? null
    : null
  const rootSpan = trace?.spans.find((span) => !span.parentSpanId) ?? trace?.spans[0] ?? null

  if (!id) {
    return <PageState title="Trace ID missing" description="Open this page with a valid trace identifier." />
  }

  if (loading && !trace) {
    return <PageState title="Loading trace" description="Fetching spans, analysis, and critical path data." />
  }

  if (error) {
    return <PageState title="Unable to load trace" description={error} actionLabel="Retry" onAction={() => void reloadTrace()} />
  }

  if (!trace) {
    return <PageState title="Trace unavailable" description="The requested trace could not be found." />
  }

  const criticalPathIds = new Set(trace.criticalPath)
  const errorSpans = trace.spans.filter((span) => span.status.code === 2)

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
        const sv = String(attr.stringValue ?? attr.intValue ?? attr.doubleValue ?? attr.boolValue ?? '').toLowerCase()
        return sv.includes(val)
      })
      if (hits) matched.add(span.spanId)
      else grayed.add(span.spanId)
    }
    return { grayedSpanIds: grayed, highlightedSpanIds: matched }
  })()

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-50px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%),radial-gradient(circle_at_bottom_right,_rgba(248,113,113,0.12),_transparent_38%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)]">
          <div className="space-y-4">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <GitBranchPlus className="h-3.5 w-3.5" />
              Trace detail
            </div>
            <div className="space-y-3">
              <div className="text-xs font-semibold uppercase tracking-[0.22em] text-muted-foreground">Trace ID</div>
              <h1 className="font-mono text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
                {id}
              </h1>
              <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
                {rootSpan?.name ?? 'Trace root unavailable'} across {trace.services.length} services, with critical path
                overlays, span filtering, and export controls for incident review.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Badge variant="outline">{trace.spanCount} spans</Badge>
              <Badge variant="outline">{trace.durationMs.toFixed(1)}ms</Badge>
              {trace.errorCount > 0 && <Badge variant="destructive">{trace.errorCount} errors</Badge>}
              {trace.services.map((service) => (
                <Badge key={service} variant="secondary" className="text-xs">
                  {service}
                </Badge>
              ))}
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Layers3 className="h-3.5 w-3.5" />
                Critical path
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{trace.criticalPath.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Spans highlighted as the dominant latency path</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <AlertTriangle className="h-3.5 w-3.5" />
                Error spans
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{errorSpans.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {rootCauseSpan ? `Root cause candidate: ${rootCauseSpan.name}` : 'No error spans detected'}
              </div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Clock3 className="h-3.5 w-3.5" />
                Operator flow
              </div>
              <div className="mt-3 flex flex-wrap gap-2 text-xs">
                <span className="rounded-full border border-border/70 bg-card px-2.5 py-1"><kbd className="font-mono">j</kbd>/<kbd className="font-mono">k</kbd> step spans</span>
                <span className="rounded-full border border-border/70 bg-card px-2.5 py-1"><kbd className="font-mono">e</kbd> export JSON</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-3">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">Investigation controls</div>
            <div className="flex flex-wrap gap-2">
              {rootCauseSpan && (
                <button
                  type="button"
                  className="rounded-full border border-red-300 bg-red-50 px-3 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-100 dark:bg-red-950/40 dark:text-red-200"
                  onClick={() => { setSelectedSpan(rootCauseSpan); setDrawerOpen(true) }}
                >
                  Root cause: {rootCauseSpan.name}
                </button>
              )}
              <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1.5 text-xs text-muted-foreground">
                Open any span to inspect metadata, events, and correlated logs
              </span>
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={copyLink}>
              <Copy className="mr-2 h-3.5 w-3.5" />
              {copied ? 'Copied!' : 'Copy link'}
            </Button>
            <Button variant="outline" size="sm" onClick={() => exportChart('svg')}>
              <Download className="mr-2 h-3.5 w-3.5" />
              Export SVG
            </Button>
            <Button variant="outline" size="sm" onClick={() => exportChart('png')}>Export PNG</Button>
            <a
              href={`/api/v1/traces/${id}/export`}
              download={`trace-${id}.json`}
            >
              <Button variant="outline" size="sm">Export JSON</Button>
            </a>
          </div>
        </div>

        <div className="mt-5 flex flex-col gap-3 lg:flex-row lg:items-center">
          <label className="flex min-w-0 flex-1 items-center gap-3 rounded-2xl border border-border/70 bg-background/70 px-4 py-3">
            <Filter className="h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              placeholder="Filter spans: key=value or key"
              value={spanFilter}
              onChange={(e) => setSpanFilter(e.target.value)}
              className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            {spanFilter && highlightedSpanIds && (
              <span className="rounded-full border border-border/70 bg-card px-2.5 py-1 text-[11px] text-muted-foreground">
                {highlightedSpanIds.size} match{highlightedSpanIds.size !== 1 ? 'es' : ''}
              </span>
            )}
          </label>
          <div className="flex rounded-2xl border border-border/70 bg-background/70 p-1 text-sm">
            <button
              type="button"
              className={`rounded-xl px-4 py-2 transition-colors ${viewMode === 'waterfall' ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:bg-accent'}`}
              onClick={() => setViewMode('waterfall')}
            >
              Waterfall
            </button>
            <button
              type="button"
              className={`rounded-xl px-4 py-2 transition-colors ${viewMode === 'flame' ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:bg-accent'}`}
              onClick={() => setViewMode('flame')}
            >
              Flame
            </button>
          </div>
        </div>

        {actionError && (
          <div className="mt-4 rounded-2xl border border-amber-300 bg-amber-50/95 px-4 py-3 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
            {actionError}
          </div>
        )}
      </section>

      <section className="overflow-hidden rounded-[28px] border border-border/70 bg-card/88 p-4 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-5">
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
      </section>

      <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
        <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1"><kbd className="font-mono">j</kbd>/<kbd className="font-mono">k</kbd> navigate spans</span>
        <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1"><kbd className="font-mono">Esc</kbd> close drawer</span>
        <span className="rounded-full border border-border/70 bg-background/60 px-2.5 py-1"><kbd className="font-mono">e</kbd> export JSON</span>
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
