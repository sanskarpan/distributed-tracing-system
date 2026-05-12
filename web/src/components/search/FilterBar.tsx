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
    <div className="flex flex-wrap gap-3 p-3 bg-muted/50 rounded-lg">
      {/* Service select */}
      <Select
        value={filters.service ?? 'all'}
        onValueChange={(v) =>
          onChange({ ...filters, service: v === 'all' ? undefined : v, operation: undefined })
        }
      >
        <SelectTrigger className="w-40" aria-label="Filter by service">
          <SelectValue placeholder="All services" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All services</SelectItem>
          {services.map(s => (
            <SelectItem key={s} value={s}>{s}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Operation select */}
      <Select
        value={filters.operation ?? 'all'}
        onValueChange={(v) =>
          onChange({ ...filters, operation: v === 'all' ? undefined : v })
        }
        disabled={!filters.service}
      >
        <SelectTrigger className="w-48" aria-label="Filter by operation">
          <SelectValue placeholder="All operations" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All operations</SelectItem>
          {operations.map(op => (
            <SelectItem key={op} value={op}>{op}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Time range select */}
      <Select
        value={String(timeRangeMin)}
        onValueChange={(v) =>
          onChange({ ...filters, timeRangeMinutes: Number(v) || undefined })
        }
      >
        <SelectTrigger className="w-36" aria-label="Filter by time range">
          <SelectValue placeholder="All time" />
        </SelectTrigger>
        <SelectContent>
          {TIME_RANGES.map(tr => (
            <SelectItem key={tr.value} value={String(tr.value)}>{tr.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Attribute search */}
      <input
        type="text"
        placeholder="attr: key=value"
        aria-label="Filter by span attribute"
        value={filters.attr ?? ''}
        onChange={(e) => onChange({ ...filters, attr: e.target.value || undefined })}
        className="h-9 rounded-md border border-input bg-background px-3 text-sm w-44 placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
      />

      {/* Errors only toggle */}
      <div className="flex items-center gap-2">
        <Switch
          checked={filters.status === 'error'}
          onCheckedChange={(checked) =>
            onChange({ ...filters, status: checked ? 'error' : 'all' })
          }
          aria-label="Show only error traces"
        />
        <span className="text-sm">Errors only</span>
      </div>

      {/* Duration range sliders */}
      <div className="flex items-center gap-3 min-w-60">
        <span className="text-xs text-muted-foreground whitespace-nowrap">Duration:</span>
        <div className="flex-1 space-y-1">
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
            <span>{minDur}ms</span>
            <span>{maxDur >= MAX_DURATION_MS ? '∞' : `${maxDur}ms`}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
