import * as d3 from 'd3'
import { useEffect, useRef, useCallback } from 'react'
import type { TraceDetailDTO, SpanDetailDTO } from '@/types'
import { getServiceColor } from '@/lib/colors'
import { buildSpanRows } from './span-layout'
import { useElementSize } from '@/hooks/useElementSize'

const ROW_HEIGHT = 22
const ROW_GAP = 2
const ROW_STRIDE = ROW_HEIGHT + ROW_GAP
const PADDING_TOP = 28

interface Props {
  trace: TraceDetailDTO
  onSpanSelect: (span: SpanDetailDTO) => void
  criticalPathIds: Set<string>
  grayedSpanIds?: Set<string>
  highlightedSpanIds?: Set<string>
}

export function FlameGraph({ trace, onSpanSelect, criticalPathIds, grayedSpanIds, highlightedSpanIds }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const onSpanSelectRef = useRef(onSpanSelect)
  const { width } = useElementSize(containerRef)

  useEffect(() => {
    onSpanSelectRef.current = onSpanSelect
  }, [onSpanSelect])

  const draw = useCallback(() => {
    if (!svgRef.current || !trace) return
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const rows = buildSpanRows(trace)
    const totalDurationMs = trace.durationMs || 1
    const svgWidth = width || containerRef.current?.clientWidth || svgRef.current.clientWidth || 1200
    const maxDepth = Math.max(...rows.map(r => r.depth), 0)
    const svgHeight = PADDING_TOP + (maxDepth + 1) * ROW_STRIDE

    svg.attr('height', svgHeight + 8)

    const xScale = d3.scaleLinear()
      .domain([0, totalDurationMs])
      .range([0, svgWidth])

    // Time axis
    const axisG = svg.append('g')
      .attr('class', 'time-axis')
      .attr('transform', `translate(0, ${PADDING_TOP})`)

    const updateAxis = (scale: d3.ScaleLinear<number, number, never>) => {
      axisG.call(
        d3.axisTop(scale)
          .tickFormat(d => `${Number(d).toFixed(0)}ms`)
          .ticks(Math.max(2, Math.floor(svgWidth / 80)))
      )
    }
    updateAxis(xScale)

    // Draw spans grouped by depth
    const spansG = svg.append('g').attr('class', 'flame-spans')

    const spanGs = spansG.selectAll<SVGGElement, typeof rows[0]>('.flame-span')
      .data(rows)
      .enter()
      .append('g')
      .attr('class', 'flame-span')
      .attr('transform', d =>
        `translate(${xScale(d.startMs)}, ${PADDING_TOP + d.depth * ROW_STRIDE})`
      )

    // Span bar
    spanGs.append('rect')
      .attr('class', 'flame-bar')
      .attr('x', 0)
      .attr('y', 0)
      .attr('width', d => Math.max(1, xScale(d.startMs + d.durationMs) - xScale(d.startMs)))
      .attr('height', ROW_HEIGHT)
      .attr('rx', 2)
      .attr('fill', d =>
        highlightedSpanIds?.has(d.spanId) ? '#22c55e' :
        grayedSpanIds?.has(d.spanId) ? '#94a3b8' :
        d.hasError ? '#dc2626' : getServiceColor(d.serviceName)
      )
      .attr('fill-opacity', d => grayedSpanIds?.has(d.spanId) ? 0.3 : 0.8)
      .attr('stroke', d => criticalPathIds.has(d.spanId) ? '#f59e0b' : 'rgba(0,0,0,0.15)')
      .attr('stroke-width', d => criticalPathIds.has(d.spanId) ? 2 : 0.5)
      .style('cursor', 'pointer')
      .on('click', (_, d) => onSpanSelectRef.current(d.span))
      .append('title')
      .text(d => `${d.serviceName}: ${d.name}\n${d.durationMs.toFixed(2)}ms`)

    // Label clipped inside bar
    spanGs.append('clipPath')
      .attr('id', d => `clip-${d.spanId}`)
      .append('rect')
      .attr('x', 2)
      .attr('y', 0)
      .attr('width', d => Math.max(0, xScale(d.startMs + d.durationMs) - xScale(d.startMs) - 4))
      .attr('height', ROW_HEIGHT)

    spanGs.append('text')
      .attr('x', 4)
      .attr('y', ROW_HEIGHT / 2)
      .attr('dominant-baseline', 'central')
      .attr('font-size', 10)
      .attr('font-family', 'monospace')
      .attr('fill', 'white')
      .attr('pointer-events', 'none')
      .attr('clip-path', d => `url(#clip-${d.spanId})`)
      .text(d => d.name)

    // Hover highlight
    spanGs.append('rect')
      .attr('class', 'flame-hitbox')
      .attr('x', 0)
      .attr('y', 0)
      .attr('width', d => Math.max(1, xScale(d.startMs + d.durationMs) - xScale(d.startMs)))
      .attr('height', ROW_HEIGHT)
      .attr('fill', 'transparent')
      .style('cursor', 'pointer')
      .on('mouseenter', function() { d3.select(this).attr('fill', 'rgba(255,255,255,0.15)') })
      .on('mouseleave', function() { d3.select(this).attr('fill', 'transparent') })
      .on('click', (_, d) => onSpanSelectRef.current(d.span))

    // Depth labels on the left (one per depth level)
    const depths = [...new Set(rows.map(r => r.depth))].sort((a, b) => a - b)
    const labelsG = svg.append('g').attr('class', 'depth-labels')
    for (const depth of depths) {
      const depthRows = rows.filter(r => r.depth === depth)
      const services = [...new Set(depthRows.map(r => r.serviceName))]
      labelsG.append('text')
        .attr('x', 4)
        .attr('y', PADDING_TOP + depth * ROW_STRIDE + ROW_HEIGHT / 2)
        .attr('dominant-baseline', 'central')
        .attr('font-size', 9)
        .attr('fill', '#94a3b8')
        .attr('pointer-events', 'none')
        .text(`d${depth}: ${services.slice(0, 2).join(', ')}${services.length > 2 ? '…' : ''}`)
    }

    // Zoom: horizontal only
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.5, 50])
      .translateExtent([[0, 0], [svgWidth, svgHeight]])
      .on('zoom', (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
        const newXScale = (event.transform as d3.ZoomTransform).rescaleX(xScale)
        updateAxis(newXScale)

        spanGs.attr('transform', d =>
          `translate(${newXScale(d.startMs)}, ${PADDING_TOP + d.depth * ROW_STRIDE})`
        )

        const barWidth = (d: typeof rows[0]) =>
          Math.max(1, newXScale(d.startMs + d.durationMs) - newXScale(d.startMs))

        spanGs.selectAll<SVGRectElement, typeof rows[0]>('.flame-bar')
          .attr('width', barWidth)

        // Update clip rects and hover rects
        spanGs.selectAll<SVGRectElement, typeof rows[0]>('clipPath rect')
          .attr('width', d => Math.max(0, barWidth(d) - 4))

        spanGs.selectAll<SVGRectElement, typeof rows[0]>('.flame-hitbox')
          .attr('width', barWidth)
      })

    svg.call(zoom)
  }, [trace, criticalPathIds, grayedSpanIds, highlightedSpanIds, width])

  useEffect(() => {
    draw()
  }, [draw])

  return (
    <div ref={containerRef} className="relative w-full overflow-hidden rounded-lg border bg-background">
      <svg ref={svgRef} className="w-full flame-svg" style={{ minHeight: 120 }} />
    </div>
  )
}
