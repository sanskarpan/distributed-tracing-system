import * as d3 from 'd3'
import { useEffect, useRef, useCallback, useState } from 'react'
import { useNavigate } from 'react-router-dom'
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

    // Group by root service — each service is one horizontal lane
    const services = [...new Set(traces.map(t => t.rootService || 'unknown'))].sort()
    const laneIndex = new Map(services.map((s, i) => [s, i]))

    // Time domain: received times + duration
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

    // Time axis
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

    // Lane backgrounds and labels
    const lanesG = svg.append('g').attr('class', 'lanes')
    services.forEach((svc, i) => {
      const y = PADDING_TOP + i * LANE_STRIDE
      // Alternating background
      lanesG.append('rect')
        .attr('x', 0)
        .attr('y', y)
        .attr('width', svgWidth)
        .attr('height', LANE_STRIDE)
        .attr('fill', i % 2 === 0 ? 'rgba(100,100,100,0.04)' : 'transparent')

      // Service label
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

    // Vertical lane separator
    lanesG.append('line')
      .attr('x1', LABEL_WIDTH)
      .attr('y1', PADDING_TOP)
      .attr('x2', LABEL_WIDTH)
      .attr('y2', svgHeight - 8)
      .attr('stroke', '#e2e8f0')
      .attr('stroke-width', 1)

    // Trace bars
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

    // Labels inside wider bars
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

    // Hover rect (full lane width for easier hit target)
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

    // Zoom
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

  return (
    <div className="p-4 max-w-6xl mx-auto">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-bold">Timeline</h1>
        <span className="text-sm text-muted-foreground">{traces.length} traces — scroll/pinch to zoom</span>
      </div>
      {loading && traces.length === 0 ? (
        <PageState title="Loading timeline" description="Fetching recent traces for the timeline view." />
      ) : error && traces.length === 0 ? (
        <PageState title="Unable to load timeline" description={error} actionLabel="Retry" onAction={loadTraces} />
      ) : traces.length === 0 ? (
        <PageState title="No traces yet" description="Ingest spans or run demo traffic to populate the timeline." />
      ) : (
        <div className="overflow-hidden rounded-lg border bg-background">
          <svg ref={svgRef} className="w-full" style={{ minHeight: 200 }} />
        </div>
      )}
    </div>
  )
}
