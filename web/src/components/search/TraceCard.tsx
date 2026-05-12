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
  const accentColor = trace.hasError ? '#dc2626' : getServiceColor(trace.rootService)

  return (
    <button
      type="button"
      className="group w-full overflow-hidden rounded-[24px] border border-border/70 bg-card/88 p-4 text-left shadow-[0_18px_70px_-42px_rgba(15,23,42,0.5)] transition-all hover:-translate-y-0.5 hover:border-primary/30 hover:bg-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      onClick={onClick}
      aria-label={`Open trace ${trace.traceId} for ${trace.rootOp}`}
    >
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className="inline-flex items-center rounded-full border border-border/70 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-foreground/80"
              style={{ backgroundColor: `${accentColor}14` }}
            >
              {trace.rootService}
            </span>
            <span className="text-xs text-muted-foreground">{trace.services.length} services involved</span>
          </div>
          <div className="text-lg font-semibold tracking-tight text-foreground transition-colors group-hover:text-primary">
            {trace.rootOp}
          </div>
        </div>

        <div className="flex items-center gap-2">
          {trace.hasError && <Badge variant="destructive">Erroring</Badge>}
          <span className="rounded-full border border-border/70 bg-background/75 px-2.5 py-1 text-xs text-muted-foreground">
            {timeAgo}
          </span>
        </div>
      </div>

      <div className="mt-4 grid gap-3 text-xs text-muted-foreground sm:grid-cols-3">
        <div className="rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em]">Span count</div>
          <div className="mt-1 text-base font-semibold text-foreground">{trace.spanCount}</div>
        </div>
        <div className="rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em]">Duration</div>
          <div className="mt-1 text-base font-semibold text-foreground">{trace.durationMs.toFixed(1)}ms</div>
        </div>
        <div className="rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em]">Trace ID</div>
          <div className="mt-1 font-mono text-[11px] text-foreground">{trace.traceId.slice(0, 16)}&hellip;</div>
        </div>
      </div>

      <div className="mt-4">
        <div className="mb-2 flex flex-wrap items-center gap-2">
          {trace.services.map(s => (
            <div
              key={s}
              className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/60 px-2.5 py-1 text-[11px] text-muted-foreground"
            >
              <span
                className="h-2.5 w-2.5 rounded-full"
                style={{ backgroundColor: getServiceColor(s) }}
                title={s}
              />
              {s}
            </div>
          ))}
        </div>
      </div>

      <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted/80">
        <div
          className="h-full rounded-full transition-all"
          style={{
            width: `${barWidth}%`,
            background: `linear-gradient(90deg, ${accentColor}, color-mix(in srgb, ${accentColor} 68%, white))`,
          }}
        />
      </div>
    </button>
  )
}

function formatTimeAgo(isoString: string): string {
  const diff = Date.now() - new Date(isoString).getTime()
  if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  return `${Math.floor(diff / 3600000)}h ago`
}
