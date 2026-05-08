import * as d3 from 'd3'
import { useEffect, useRef, useCallback } from 'react'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'
import { getServiceColor } from '@/lib/colors'

const LABEL_WIDTH = 220
const ROW_HEIGHT = 24
const ROW_GAP = 4
const ROW_STRIDE = ROW_HEIGHT + ROW_GAP
const PADDING_TOP = 32
const MINIMAP_H = 80

interface SpanRow {
  spanId: string
  name: string
  serviceName: string
  startMs: number
  durationMs: number
  depth: number
  hasError: boolean
  kind: number
  span: SpanDetailDTO
}

interface Props {
  trace: TraceDetailDTO
  onSpanSelect: (span: SpanDetailDTO) => void
  criticalPathIds: Set<string>
  grayedSpanIds?: Set<string>
  highlightedSpanIds?: Set<string>
  durationDeltas?: Map<string, number>
}

export function buildSpanRows(trace: TraceDetailDTO): SpanRow[] {
  const rows: SpanRow[] = []
  const traceStart = Math.min(...trace.spans.map(s => s.startTimeUnixNano)) / 1e6

  function visit(span: SpanDetailDTO, depth: number) {
    rows.push({
      spanId: span.spanId,
      name: span.name,
      serviceName: span.serviceName,
      startMs: span.startTimeUnixNano / 1e6 - traceStart,
      durationMs: span.durationMs,
      depth,
      hasError: span.status.code === 2,
      kind: span.kind,
      span,
    })
    const children = trace.spans
      .filter(s => s.parentSpanId === span.spanId)
      .sort((a, b) => a.startTimeUnixNano - b.startTimeUnixNano)
    for (const child of children) {
      visit(child, depth + 1)
    }
  }

  const root = trace.spans.find(
    s => !s.parentSpanId || s.parentSpanId === '0000000000000000'
  )
  if (root) visit(root, 0)

  const visited = new Set(rows.map(r => r.spanId))
  for (const s of trace.spans) {
    if (!visited.has(s.spanId)) {
      rows.push({
        spanId: s.spanId,
        name: s.name,
        serviceName: s.serviceName,
        startMs: s.startTimeUnixNano / 1e6 - traceStart,
        durationMs: s.durationMs,
        depth: 0,
        hasError: s.status.code === 2,
        kind: s.kind,
        span: s,
      })
    }
  }

  return rows
}

export function WaterfallChart({ trace, onSpanSelect, criticalPathIds, grayedSpanIds, highlightedSpanIds, durationDeltas }: Props) {
  const svgRef = useRef<SVGSVGElement>(null)

  const onSpanSelectRef = useRef(onSpanSelect)
  onSpanSelectRef.current = onSpanSelect

  const draw = useCallback(() => {
    if (!svgRef.current || !trace) return
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const rows = buildSpanRows(trace)
    const totalDurationMs = trace.durationMs || 1
    const svgWidth = svgRef.current.clientWidth || 1200
    const chartWidth = svgWidth - LABEL_WIDTH
    const svgHeight = PADDING_TOP + rows.length * ROW_STRIDE

    svg.attr('height', svgHeight + MINIMAP_H + 16)

    const xScale = d3.scaleLinear()
      .domain([0, totalDurationMs])
      .range([LABEL_WIDTH, svgWidth])

    // Time axis
    const axisG = svg.append('g')
      .attr('class', 'time-axis')
      .attr('transform', `translate(0, ${PADDING_TOP})`)

    const updateAxis = (scale: d3.ScaleLinear<number, number, never>) => {
      axisG.call(
        d3.axisTop(scale)
          .tickFormat(d => `${Number(d).toFixed(0)}ms`)
          .ticks(Math.max(2, Math.floor(chartWidth / 80)))
      )
    }
    updateAxis(xScale)

    const rowsG = svg.append('g').attr('class', 'span-rows')

    const rowGs = rowsG.selectAll<SVGGElement, SpanRow>('.span-row')
      .data(rows)
      .enter()
      .append('g')
      .attr('class', d => `span-row${d.hasError ? ' has-error' : ''}`)
      .attr('transform', (_, i) => `translate(0, ${PADDING_TOP + i * ROW_STRIDE})`)

    // Service label (left side)
    rowGs.append('text')
      .attr('x', d => Math.min(LABEL_WIDTH - 8, LABEL_WIDTH - 8 + d.depth * 10))
      .attr('y', ROW_HEIGHT / 2)
      .attr('text-anchor', 'end')
      .attr('dominant-baseline', 'central')
      .attr('fill', d => getServiceColor(d.serviceName))
      .attr('font-size', 11)
      .attr('font-family', 'monospace')
      .text(d => {
        const label = d.name.length > 22 ? d.name.slice(0, 22) + '\u2026' : d.name
        return '\u00a0\u00a0'.repeat(Math.min(d.depth, 3)) + label
      })

    // Tree connector line
    rowGs.filter(d => d.depth > 0)
      .append('line')
      .attr('x1', d => LABEL_WIDTH - 8 + (d.depth - 1) * 10 + 4)
      .attr('y1', 0)
      .attr('x2', d => LABEL_WIDTH - 8 + (d.depth - 1) * 10 + 4)
      .attr('y2', ROW_HEIGHT / 2)
      .attr('stroke', '#94a3b8')
      .attr('stroke-width', 1)

    // Span bar background
    rowGs.append('rect')
      .attr('class', 'span-bg')
      .attr('x', d => xScale(d.startMs))
      .attr('y', 2)
      .attr('width', d => Math.max(2, xScale(d.startMs + d.durationMs) - xScale(d.startMs)))
      .attr('height', ROW_HEIGHT - 4)
      .attr('rx', 3)
      .attr('fill', d =>
        highlightedSpanIds?.has(d.spanId) ? '#22c55e' :
        grayedSpanIds?.has(d.spanId) ? '#94a3b8' :
        d.hasError ? '#dc2626' : getServiceColor(d.serviceName)
      )
      .attr('fill-opacity', d => grayedSpanIds?.has(d.spanId) ? 0.3 : d.hasError ? 0.9 : 0.7)
      .attr('stroke', d => criticalPathIds.has(d.spanId) ? '#f59e0b' : 'none')
      .attr('stroke-width', 2)
      .style('cursor', 'pointer')
      .append('title')
      .text(d => `${d.serviceName}: ${d.name}\n${d.durationMs.toFixed(2)}ms`)

    // Re-select to add click (can't chain after append title)
    rowGs.selectAll<SVGRectElement, SpanRow>('.span-bg')
      .on('click', (_, d) => onSpanSelectRef.current(d.span))

    // Duration label inside bar
    rowGs.append('text')
      .attr('class', 'duration-label')
      .attr('x', d => xScale(d.startMs) + 4)
      .attr('y', ROW_HEIGHT / 2)
      .attr('dominant-baseline', 'central')
      .attr('font-size', 10)
      .attr('fill', 'white')
      .attr('pointer-events', 'none')
      .text(d => {
        const barW = xScale(d.startMs + d.durationMs) - xScale(d.startMs)
        return barW > 60 ? `${d.durationMs.toFixed(1)}ms` : ''
      })

    // Duration delta badge (for Compare page diff overlay)
    if (durationDeltas) {
      rowGs.append('text')
        .attr('class', 'delta-badge')
        .attr('x', d => xScale(d.startMs + d.durationMs) + 4)
        .attr('y', ROW_HEIGHT / 2)
        .attr('dominant-baseline', 'central')
        .attr('font-size', 9)
        .attr('font-weight', 600)
        .attr('pointer-events', 'none')
        .attr('fill', d => {
          const delta = durationDeltas.get(d.spanId)
          if (delta === undefined) return 'transparent'
          return delta > 0 ? '#dc2626' : '#16a34a'
        })
        .text(d => {
          const delta = durationDeltas.get(d.spanId)
          if (delta === undefined || Math.abs(delta) < 0.5) return ''
          return delta > 0 ? `+${delta.toFixed(1)}ms` : `${delta.toFixed(1)}ms`
        })
    }

    // Row background for hover
    rowGs.append('rect')
      .attr('x', 0)
      .attr('y', 0)
      .attr('width', svgWidth)
      .attr('height', ROW_STRIDE)
      .attr('fill', 'transparent')
      .style('cursor', 'pointer')
      .on('mouseenter', function() { d3.select(this).attr('fill', 'rgba(100,100,100,0.05)') })
      .on('mouseleave', function() { d3.select(this).attr('fill', 'transparent') })
      .on('click', (_, d) => onSpanSelectRef.current(d.span))

    // Zoom behavior
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.5, 50])
      .translateExtent([[LABEL_WIDTH, 0], [svgWidth, svgHeight]])
      .on('zoom', (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
        const newXScale = (event.transform as d3.ZoomTransform).rescaleX(xScale)
        updateAxis(newXScale)

        rowGs.selectAll<SVGRectElement, SpanRow>('.span-bg')
          .attr('x', d => newXScale(d.startMs))
          .attr('width', d => Math.max(2, newXScale(d.startMs + d.durationMs) - newXScale(d.startMs)))

        rowGs.selectAll<SVGTextElement, SpanRow>('.duration-label')
          .attr('x', d => newXScale(d.startMs) + 4)
          .text(d => {
            const barW = newXScale(d.startMs + d.durationMs) - newXScale(d.startMs)
            return barW > 60 ? `${d.durationMs.toFixed(1)}ms` : ''
          })

        rowGs.selectAll<SVGTextElement, SpanRow>('.delta-badge')
          .attr('x', d => newXScale(d.startMs + d.durationMs) + 4)
      })

    svg.call(zoom)

    // Minimap
    const minimapG = svg.append('g')
      .attr('transform', `translate(${LABEL_WIDTH}, ${svgHeight + 8})`)

    const mmXScale = d3.scaleLinear()
      .domain([0, totalDurationMs])
      .range([0, chartWidth])

    const mmYScale = d3.scaleLinear()
      .domain([0, rows.length])
      .range([0, MINIMAP_H - 8])

    minimapG.selectAll<SVGRectElement, SpanRow>('.mm-bar')
      .data(rows)
      .enter()
      .append('rect')
      .attr('x', d => mmXScale(d.startMs))
      .attr('y', (_, i) => mmYScale(i))
      .attr('width', d => Math.max(1, mmXScale(d.startMs + d.durationMs) - mmXScale(d.startMs)))
      .attr('height', Math.max(1, (MINIMAP_H - 8) / Math.max(rows.length, 1) - 1))
      .attr('fill', d => d.hasError ? '#dc2626' : getServiceColor(d.serviceName))
      .attr('fill-opacity', 0.6)
  }, [trace, criticalPathIds, grayedSpanIds, highlightedSpanIds, durationDeltas])

  useEffect(() => {
    draw()
  }, [draw])

  return (
    <div className="relative w-full overflow-hidden border rounded-lg bg-white">
      <svg ref={svgRef} className="w-full waterfall-svg" style={{ minHeight: 200 }} />
    </div>
  )
}
