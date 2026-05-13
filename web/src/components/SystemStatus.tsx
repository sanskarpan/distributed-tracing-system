import { Activity, AlertTriangle, Gauge, LoaderCircle } from 'lucide-react'
import { useCollectorStatus } from '@/hooks/useCollectorStatus'

function statusTone(status: string | null, hasError: boolean) {
  if (hasError) {
    return {
      dot: 'bg-red-500',
      label: 'Collector offline',
      detail: 'Unable to reach /readyz',
      icon: AlertTriangle,
      panel: 'border-red-300/70 bg-red-50/80 text-red-900 dark:bg-red-950/30 dark:text-red-100',
    }
  }

  switch (status) {
    case 'overloaded':
      return {
        dot: 'bg-amber-500',
        label: 'Collector overloaded',
        detail: 'Queue pressure is above the configured readiness threshold',
        icon: Gauge,
        panel: 'border-amber-300/70 bg-amber-50/80 text-amber-950 dark:bg-amber-950/30 dark:text-amber-100',
      }
    case 'draining':
      return {
        dot: 'bg-orange-500',
        label: 'Collector draining',
        detail: 'Readiness is intentionally disabled while the process winds down',
        icon: LoaderCircle,
        panel: 'border-orange-300/70 bg-orange-50/80 text-orange-950 dark:bg-orange-950/30 dark:text-orange-100',
      }
    default:
      return {
        dot: 'bg-emerald-500',
        label: 'Collector ready',
        detail: 'Healthy and ready to accept traffic',
        icon: Activity,
        panel: 'border-emerald-300/70 bg-emerald-50/80 text-emerald-950 dark:bg-emerald-950/30 dark:text-emerald-100',
      }
  }
}

export function SystemStatus() {
  const { status, error } = useCollectorStatus()
  const tone = statusTone(status?.status ?? null, Boolean(error))
  const Icon = tone.icon
  const isSpinning = status?.status === 'draining'
  const summary = error ?? tone.detail
  const compactDetails =
    status && !error
      ? `queue ${status.queueDepth}/${status.queueCapacity || 0} • ${Math.round(status.uptimeSec)}s uptime`
      : summary
  const stats = status && !error
    ? [
        `${Math.round(status.uptimeSec)}s uptime`,
        `${status.heapMB} MB heap`,
        `${status.goroutines} goroutines`,
        `queue ${status.queueDepth}/${status.queueCapacity || 0}`,
      ]
    : []

  return (
    <>
      <div
        data-testid="collector-status-mobile"
        className={`inline-flex items-center gap-3 rounded-2xl border px-3 py-2 shadow-sm backdrop-blur xl:hidden ${tone.panel}`}
      >
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-current/10 bg-background/60">
          <Icon className={`h-4 w-4 ${isSpinning ? 'animate-spin' : ''}`} />
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <div className={`h-2.5 w-2.5 rounded-full ${tone.dot}`} />
            <div className="text-sm font-semibold">{tone.label}</div>
          </div>
          <div className="mt-0.5 text-[11px] leading-4 opacity-80">
            {compactDetails}
          </div>
        </div>
      </div>

      <div
        data-testid="collector-status-desktop"
        className={`hidden min-w-[280px] rounded-2xl border px-3 py-2 shadow-sm backdrop-blur xl:block ${tone.panel}`}
      >
        <div className="flex items-start gap-3">
          <div className={`mt-1 h-2.5 w-2.5 rounded-full ${tone.dot}`} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <Icon className={`h-4 w-4 ${isSpinning ? 'animate-spin' : ''}`} />
              <div className="text-sm font-semibold">{tone.label}</div>
            </div>
            <div className="mt-1 text-xs leading-5 opacity-80">{summary}</div>
            {stats.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-2 text-[11px] uppercase tracking-[0.18em] opacity-80">
                {stats.map(stat => <span key={stat}>{stat}</span>)}
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  )
}
