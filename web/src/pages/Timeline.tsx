import * as d3 from 'd3'
import { useEffect, useRef, useCallback, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Clock3, ScanSearch, Waves } from 'lucide-react'
import { api } from '@/api/client'
import { useSSE } from '@/hooks/useSSE'
import { getServiceColor } from '@/lib/colors'
import type { TraceSummaryDTO } from '@/types'
import { PageState } from '@/components/ui/page-state'
import { getErrorMessage } from '@/lib/errors'

const LANE_HEIGHT = 28
const LANE_GAP = 6
const LANE_STRIDE = LANE_HEIGHT + LANE_GAP
const LABEL_WIDTH = 160
const PADDING_TOP = 32

export function TimelinePage() {
  const navigate = useNavigate()
  const svgRef = useRef<SVGSVGElement>(null)
  const [traces, setTraces] = useState<TraceSummaryDTO[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadTraces = useCallback(() => {
    setError(null)
    api.getTraces({ limit: 200, sortBy: 'receivedAt', sortDesc: 'false' })
      .then(r => setTraces(r.traces))
      .catch((err: unknown) => setError(getErrorMessage(err, 'Failed to load timeline traces.')))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(loadTraces, 0)
    return () => window.clearTimeout(timer)
  }, [loadTraces])

  useSSE('/sse/traces', (event: unknown) => {
    const e = event as { type?: string }
    if (e.type === 'trace') {
      loadTraces()
    }
  })

  const draw = useCallback(() => {
    if (!svgRef.current || traces.length === 0) return
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const services = [...new Set(traces.map(t => t.rootService || 'unknown'))].sort()
    const laneIndex = new Map(services.map((s, i) => [s, i]))
    const times = traces.map(t => new Date(t.receivedAt).getTime())
    const minTime = Math.min(...times)
    const maxTime = Math.max(...times.map((t, i) => t + traces[i].durationMs))

    const svgWidth = svgRef.current.clientWidth || 1200
    const chartWidth = svgWidth - LABEL_WIDTH
    const svgHeight = PADDING_TOP + services.length * LANE_STRIDE + 16

    svg.attr('height', svgHeight)

    const xScale = d3.scaleLinear()
      .domain([minTime, Math.max(maxTime, minTime + 1)])
      .range([LABEL_WIDTH, svgWidth])

    const axisG = svg.append('g')
      .attr('class', 'time-axis')
      .attr('transform', `translate(0, ${PADDING_TOP})`)

    const updateAxis = (scale: d3.ScaleLinear<number, number, never>) => {
      axisG.call(
        d3.axisTop(scale)
          .tickFormat(d => {
            const dt = new Date(Number(d))
            return `${dt.getHours().toString().padStart(2, '0')}:${dt.getMinutes().toString().padStart(2, '0')}:${dt.getSeconds().toString().padStart(2, '0')}`
          })
          .ticks(Math.max(2, Math.floor(chartWidth / 120)))
      )
    }
    updateAxis(xScale)

    const lanesG = svg.append('g').attr('class', 'lanes')
    services.forEach((svc, i) => {
      const y = PADDING_TOP + i * LANE_STRIDE

      lanesG.append('rect')
        .attr('x', 0)
        .attr('y', y)
        .attr('width', svgWidth)
        .attr('height', LANE_STRIDE)
        .attr('fill', i % 2 === 0 ? 'rgba(100,100,100,0.04)' : 'transparent')

      lanesG.append('text')
        .attr('x', LABEL_WIDTH - 8)
        .attr('y', y + LANE_HEIGHT / 2)
        .attr('text-anchor', 'end')
        .attr('dominant-baseline', 'central')
        .attr('font-size', 11)
        .attr('font-family', 'monospace')
        .attr('fill', getServiceColor(svc))
        .text(svc.length > 18 ? svc.slice(0, 18) + '\u2026' : svc)
    })

    lanesG.append('line')
      .attr('x1', LABEL_WIDTH)
      .attr('y1', PADDING_TOP)
      .attr('x2', LABEL_WIDTH)
      .attr('y2', svgHeight - 8)
      .attr('stroke', '#e2e8f0')
      .attr('stroke-width', 1)

    const tracesG = svg.append('g').attr('class', 'trace-bars')
    const traceGs = tracesG.selectAll<SVGGElement, TraceSummaryDTO>('.trace-bar')
      .data(traces)
      .enter()
      .append('g')
      .attr('class', 'trace-bar')

    traceGs.append('rect')
      .attr('class', 'bar')
      .attr('x', d => xScale(new Date(d.receivedAt).getTime()))
      .attr('y', d => {
        const lane = laneIndex.get(d.rootService || 'unknown') ?? 0
        return PADDING_TOP + lane * LANE_STRIDE + 3
      })
      .attr('width', d => Math.max(3, xScale(new Date(d.receivedAt).getTime() + d.durationMs) - xScale(new Date(d.receivedAt).getTime())))
      .attr('height', LANE_HEIGHT - 6)
      .attr('rx', 3)
      .attr('fill', d => d.hasError ? '#dc2626' : getServiceColor(d.rootService || 'unknown'))
      .attr('fill-opacity', 0.75)
      .style('cursor', 'pointer')
      .on('click', (_, d) => navigate(`/trace/${d.traceId}`))
      .append('title')
      .text(d => `${d.rootService}: ${d.rootOp}\n${d.durationMs.toFixed(1)}ms — ${d.spanCount} spans\n${new Date(d.receivedAt).toISOString()}`)

    traceGs.append('text')
      .attr('x', d => xScale(new Date(d.receivedAt).getTime()) + 4)
      .attr('y', d => {
        const lane = laneIndex.get(d.rootService || 'unknown') ?? 0
        return PADDING_TOP + lane * LANE_STRIDE + LANE_HEIGHT / 2
      })
      .attr('dominant-baseline', 'central')
      .attr('font-size', 9)
      .attr('font-family', 'monospace')
      .attr('fill', 'white')
      .attr('pointer-events', 'none')
      .text(d => {
        const barW = xScale(new Date(d.receivedAt).getTime() + d.durationMs) - xScale(new Date(d.receivedAt).getTime())
        if (barW < 50) return ''
        const label = d.rootOp || ''
        return label.length > 20 ? label.slice(0, 20) + '\u2026' : label
      })

    traceGs.append('rect')
      .attr('x', d => xScale(new Date(d.receivedAt).getTime()) - 1)
      .attr('y', d => {
        const lane = laneIndex.get(d.rootService || 'unknown') ?? 0
        return PADDING_TOP + lane * LANE_STRIDE
      })
      .attr('width', d => Math.max(5, xScale(new Date(d.receivedAt).getTime() + d.durationMs) - xScale(new Date(d.receivedAt).getTime()) + 2))
      .attr('height', LANE_HEIGHT)
      .attr('fill', 'transparent')
      .style('cursor', 'pointer')
      .on('mouseenter', function() { d3.select(this).attr('fill', 'rgba(255,255,255,0.1)') })
      .on('mouseleave', function() { d3.select(this).attr('fill', 'transparent') })
      .on('click', (_, d) => navigate(`/trace/${d.traceId}`))

    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.5, 200])
      .translateExtent([[LABEL_WIDTH, 0], [svgWidth, svgHeight]])
      .on('zoom', (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
        const newXScale = (event.transform as d3.ZoomTransform).rescaleX(xScale)
        updateAxis(newXScale)

        const x = (d: TraceSummaryDTO) => newXScale(new Date(d.receivedAt).getTime())
        const w = (d: TraceSummaryDTO) =>
          Math.max(3, newXScale(new Date(d.receivedAt).getTime() + d.durationMs) - newXScale(new Date(d.receivedAt).getTime()))

        traceGs.selectAll<SVGRectElement, TraceSummaryDTO>('.bar')
          .attr('x', x)
          .attr('width', w)

        traceGs.selectAll<SVGTextElement, TraceSummaryDTO>('text')
          .attr('x', d => newXScale(new Date(d.receivedAt).getTime()) + 4)
          .text(d => {
            const barW = w(d)
            if (barW < 50) return ''
            const label = d.rootOp || ''
            return label.length > 20 ? label.slice(0, 20) + '\u2026' : label
          })

        traceGs.selectAll<SVGRectElement, TraceSummaryDTO>('rect:last-child')
          .attr('x', d => newXScale(new Date(d.receivedAt).getTime()) - 1)
          .attr('width', d => Math.max(5, w(d) + 2))
      })

    svg.call(zoom)
  }, [traces, navigate])

  useEffect(() => { draw() }, [draw])

  const services = [...new Set(traces.map((trace) => trace.rootService || 'unknown'))]

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-50px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)]">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <Waves className="h-3.5 w-3.5" />
              Temporal view
            </div>
            <h1 className="text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
              Watch traces accumulate across services as the incident unfolds.
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
              The timeline compresses recent trace arrivals into service lanes so you can spot bursts, stalls, and
              latency clusters without drilling into a single span tree first.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <ScanSearch className="h-3.5 w-3.5" />
                Lanes
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{services.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Distinct root services represented in the window</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Clock3 className="h-3.5 w-3.5" />
                Trace volume
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{traces.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Scroll or pinch inside the chart to rescale time</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Interaction</div>
              <div className="mt-3 text-sm font-medium text-foreground">Click any bar to open the full trace detail view.</div>
            </div>
          </div>
        </div>
      </section>

      {loading && traces.length === 0 ? (
        <PageState title="Loading timeline" description="Fetching recent traces for the timeline view." />
      ) : error && traces.length === 0 ? (
        <PageState title="Unable to load timeline" description={error} actionLabel="Retry" onAction={loadTraces} />
      ) : traces.length === 0 ? (
        <PageState title="No traces yet" description="Ingest spans or run demo traffic to populate the timeline." />
      ) : (
        <section className="overflow-hidden rounded-[28px] border border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/70 px-5 py-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Timeline canvas</div>
              <div className="mt-1 text-sm text-muted-foreground">Service lanes ordered by root service, with each bar positioned by received time.</div>
            </div>
            <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1.5 text-sm text-muted-foreground">
              {traces.length} traces across {services.length} service lane{services.length !== 1 ? 's' : ''}
            </span>
          </div>
          <div className="p-4">
            <svg ref={svgRef} className="timeline-svg w-full" style={{ minHeight: 280 }} />
          </div>
        </section>
      )}
    </div>
  )
}
