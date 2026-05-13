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

  return (
    <div className={`hidden min-w-[280px] rounded-2xl border px-3 py-2 shadow-sm backdrop-blur xl:block ${tone.panel}`}>
      <div className="flex items-start gap-3">
        <div className={`mt-1 h-2.5 w-2.5 rounded-full ${tone.dot}`} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <Icon className={`h-4 w-4 ${status?.status === 'draining' ? 'animate-spin' : ''}`} />
            <div className="text-sm font-semibold">{tone.label}</div>
          </div>
          <div className="mt-1 text-xs leading-5 opacity-80">
            {error ?? tone.detail}
          </div>
          {status && !error && (
            <div className="mt-2 flex flex-wrap gap-2 text-[11px] uppercase tracking-[0.18em] opacity-80">
              <span>{Math.round(status.uptimeSec)}s uptime</span>
              <span>{status.heapMB} MB heap</span>
              <span>{status.goroutines} goroutines</span>
              <span>
                queue {status.queueDepth}/{status.queueCapacity || 0}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
