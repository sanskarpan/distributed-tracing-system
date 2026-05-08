import * as d3 from 'd3'
import { useEffect, useRef } from 'react'
import type { LatencyHeatmapData } from '@/types'

interface Props {
  data: LatencyHeatmapData
}

const CELL_W = 48
const CELL_H = 20
const LABEL_W = 70
const LABEL_H = 32

export function LatencyHeatmapChart({ data }: Props) {
  const svgRef = useRef<SVGSVGElement>(null)

  useEffect(() => {
    if (!svgRef.current || data.buckets.length === 0) return
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const numTimes = data.buckets.length
    const numBands = data.bounds.length + 1 // +1 for overflow

    const svgW = LABEL_W + numTimes * CELL_W
    const svgH = LABEL_H + numBands * CELL_H

    svg.attr('width', svgW).attr('height', svgH)

    // Find max count for color scale
    let maxCount = 1
    for (const b of data.buckets) {
      for (const c of b.counts) {
        if (c > maxCount) maxCount = c
      }
    }

    const colorScale = d3.scaleSequential(d3.interpolateYlOrRd)
      .domain([0, maxCount])

    const g = svg.append('g').attr('transform', `translate(${LABEL_W},${LABEL_H})`)

    // Cells
    for (let ti = 0; ti < numTimes; ti++) {
      const bucket = data.buckets[ti]
      for (let bi = 0; bi < numBands; bi++) {
        const count = bucket.counts[bi] ?? 0
        g.append('rect')
          .attr('x', ti * CELL_W)
          .attr('y', (numBands - 1 - bi) * CELL_H) // highest latency at top
          .attr('width', CELL_W - 1)
          .attr('height', CELL_H - 1)
          .attr('fill', count === 0 ? '#f1f5f9' : colorScale(count))
          .attr('rx', 1)
          .append('title')
          .text(() => {
            const low = bi === 0 ? 0 : data.bounds[bi - 1]
            const high = bi < data.bounds.length ? data.bounds[bi] : '∞'
            const ts = new Date(bucket.ts * 1000)
            return `${ts.toLocaleTimeString()}: ${count} spans (${low}–${high}ms)`
          })

        // Count label inside cell
        if (count > 0) {
          g.append('text')
            .attr('x', ti * CELL_W + CELL_W / 2)
            .attr('y', (numBands - 1 - bi) * CELL_H + CELL_H / 2)
            .attr('text-anchor', 'middle')
            .attr('dominant-baseline', 'central')
            .attr('font-size', 9)
            .attr('fill', count > maxCount * 0.6 ? 'white' : '#374151')
            .attr('pointer-events', 'none')
            .text(count > 999 ? `${(count / 1000).toFixed(1)}k` : String(count))
        }
      }
    }

    // Y-axis labels (latency bands)
    const labelsG = svg.append('g')
    for (let bi = 0; bi < numBands; bi++) {
      const label = bi === 0 ? `≤${data.bounds[0]}ms`
        : bi < data.bounds.length ? `${data.bounds[bi - 1]}–${data.bounds[bi]}ms`
        : `>${data.bounds[data.bounds.length - 1]}ms`
      labelsG.append('text')
        .attr('x', LABEL_W - 4)
        .attr('y', LABEL_H + (numBands - 1 - bi) * CELL_H + CELL_H / 2)
        .attr('text-anchor', 'end')
        .attr('dominant-baseline', 'central')
        .attr('font-size', 9)
        .attr('font-family', 'monospace')
        .attr('fill', '#64748b')
        .text(label)
    }

    // X-axis labels (time)
    for (let ti = 0; ti < numTimes; ti++) {
      const ts = new Date(data.buckets[ti].ts * 1000)
      const label = `${ts.getHours().toString().padStart(2, '0')}:${ts.getMinutes().toString().padStart(2, '0')}:${ts.getSeconds().toString().padStart(2, '0')}`
      svg.append('text')
        .attr('x', LABEL_W + ti * CELL_W + CELL_W / 2)
        .attr('y', LABEL_H - 4)
        .attr('text-anchor', 'middle')
        .attr('font-size', 9)
        .attr('font-family', 'monospace')
        .attr('fill', '#64748b')
        .text(label)
    }
  }, [data])

  if (data.buckets.length === 0) {
    return <p className="text-sm text-muted-foreground text-center py-4">No latency data yet.</p>
  }

  return (
    <div className="overflow-x-auto">
      <svg ref={svgRef} />
    </div>
  )
}
