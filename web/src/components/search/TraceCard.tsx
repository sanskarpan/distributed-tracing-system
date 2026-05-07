import { Badge } from '@/components/ui/badge'
import type { TraceSummaryDTO } from '@/types'
import { getServiceColor } from '@/lib/colors'

interface Props {
  trace: TraceSummaryDTO
  maxDuration: number
  onClick: () => void
}

export function TraceCard({ trace, maxDuration, onClick }: Props) {
  const barWidth = maxDuration > 0 ? (trace.durationMs / maxDuration) * 100 : 0
  const timeAgo = formatTimeAgo(trace.receivedAt)

  return (
    <div
      className="border rounded-lg p-3 hover:bg-accent cursor-pointer transition-colors"
      onClick={onClick}
    >
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          {trace.services.map(s => (
            <div
              key={s}
              className="w-3 h-3 rounded-full flex-shrink-0"
              style={{ backgroundColor: getServiceColor(s) }}
              title={s}
            />
          ))}
          <span className="text-sm font-medium">{trace.rootOp}</span>
        </div>
        <div className="flex items-center gap-2">
          {trace.hasError && <Badge variant="destructive">ERROR</Badge>}
          <span className="text-xs text-muted-foreground">{timeAgo}</span>
        </div>
      </div>

      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span>{trace.spanCount} spans</span>
        <span>{trace.durationMs.toFixed(1)}ms</span>
        <span className="font-mono text-[10px]">{trace.traceId.slice(0, 16)}&hellip;</span>
      </div>

      <div className="mt-2 h-1.5 bg-muted rounded-full overflow-hidden">
        <div
          className="h-full rounded-full transition-all"
          style={{
            width: `${barWidth}%`,
            backgroundColor: trace.hasError ? '#dc2626' : getServiceColor(trace.rootService),
          }}
        />
      </div>
    </div>
  )
}

function formatTimeAgo(isoString: string): string {
  const diff = Date.now() - new Date(isoString).getTime()
  if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  return `${Math.floor(diff / 3600000)}h ago`
}
