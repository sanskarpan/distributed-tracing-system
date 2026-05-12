import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Slider } from '@/components/ui/slider'
import type { SearchFilters } from '@/hooks/useSearch'

interface Props {
  services: string[]
  operations: string[]
  filters: SearchFilters
  onChange: (filters: SearchFilters) => void
}

const TIME_RANGES = [
  { label: 'Last 15m', value: 15 },
  { label: 'Last 1h', value: 60 },
  { label: 'Last 6h', value: 360 },
  { label: 'Last 24h', value: 1440 },
  { label: 'Last 7d', value: 10080 },
  { label: 'All time', value: 0 },
]

const MAX_DURATION_MS = 30000

export function FilterBar({ services, operations, filters, onChange }: Props) {
  const minDur = filters.minDuration ?? 0
  const maxDur = filters.maxDuration ?? MAX_DURATION_MS
  const timeRangeMin = filters.timeRangeMinutes ?? 0

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-[1fr_1fr_0.8fr_1fr_1.4fr]">
      <div className="space-y-2 rounded-2xl border border-border/70 bg-card/75 p-3 shadow-sm">
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Service</div>
        <Select
          value={filters.service ?? 'all'}
          onValueChange={(v) =>
            onChange({ ...filters, service: v === 'all' ? undefined : v, operation: undefined })
          }
        >
          <SelectTrigger className="w-full bg-background/70" aria-label="Filter by service">
            <SelectValue placeholder="All services" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All services</SelectItem>
            {services.map(s => (
              <SelectItem key={s} value={s}>{s}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 rounded-2xl border border-border/70 bg-card/75 p-3 shadow-sm">
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Operation</div>
        <Select
          value={filters.operation ?? 'all'}
          onValueChange={(v) =>
            onChange({ ...filters, operation: v === 'all' ? undefined : v })
          }
          disabled={!filters.service}
        >
          <SelectTrigger className="w-full bg-background/70" aria-label="Filter by operation">
            <SelectValue placeholder="All operations" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All operations</SelectItem>
            {operations.map(op => (
              <SelectItem key={op} value={op}>{op}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 rounded-2xl border border-border/70 bg-card/75 p-3 shadow-sm">
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Time window</div>
        <Select
          value={String(timeRangeMin)}
          onValueChange={(v) =>
            onChange({ ...filters, timeRangeMinutes: Number(v) || undefined })
          }
        >
          <SelectTrigger className="w-full bg-background/70" aria-label="Filter by time range">
            <SelectValue placeholder="All time" />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGES.map(tr => (
              <SelectItem key={tr.value} value={String(tr.value)}>{tr.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 rounded-2xl border border-border/70 bg-card/75 p-3 shadow-sm">
        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Span attribute</div>
        <input
          type="text"
          placeholder="key=value"
          aria-label="Filter by span attribute"
          value={filters.attr ?? ''}
          onChange={(e) => onChange({ ...filters, attr: e.target.value || undefined })}
          className="h-10 w-full rounded-xl border border-input bg-background/70 px-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
        />
        <div className="text-xs text-muted-foreground">Match trace spans by attribute key or `key=value`.</div>
      </div>

      <div className="space-y-3 rounded-2xl border border-border/70 bg-card/75 p-3 shadow-sm">
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Latency & failures</div>
            <div className="text-xs text-muted-foreground">Trim noisy traces and focus on outliers.</div>
          </div>
          <div className="flex items-center gap-2 rounded-full border border-border/70 bg-background/60 px-3 py-1.5">
            <Switch
              checked={filters.status === 'error'}
              onCheckedChange={(checked) =>
                onChange({ ...filters, status: checked ? 'error' : 'all' })
              }
              aria-label="Show only error traces"
            />
            <span className="text-sm">Errors</span>
          </div>
        </div>

        <div className="space-y-2">
          <Slider
            value={[minDur, maxDur]}
            min={0}
            max={MAX_DURATION_MS}
            step={100}
            onValueChange={([min, max]) =>
              onChange({
                ...filters,
                minDuration: min > 0 ? min : undefined,
                maxDuration: max < MAX_DURATION_MS ? max : undefined,
              })
            }
          />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>{minDur}ms floor</span>
            <span>{maxDur >= MAX_DURATION_MS ? 'No cap' : `${maxDur}ms cap`}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
