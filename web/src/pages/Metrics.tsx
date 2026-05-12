import { useEffect, useState, useCallback, useMemo } from 'react'
import {
  LineChart,
  Line,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  BarChart,
  Bar,
} from 'recharts'
import { AlertTriangle, Gauge, ShieldAlert, Waves, Waypoints } from 'lucide-react'
import { api } from '@/api/client'
import { useSSE } from '@/hooks/useSSE'
import { LatencyHeatmapChart } from '@/components/metrics/LatencyHeatmapChart'
import type { MetricSnapshotDTO, AnomalyResult, SLOResult, LatencyHeatmapData } from '@/types'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PageState } from '@/components/ui/page-state'
import { getErrorMessage } from '@/lib/errors'

type SortKey = 'operation' | 'rate' | 'errorRate' | 'p50Ms' | 'p95Ms' | 'p99Ms'

export function MetricsPage() {
  const [metrics, setMetrics] = useState<MetricSnapshotDTO[]>([])
  const [selectedService, setSelectedService] = useState<string>('')
  const [sortKey, setSortKey] = useState<SortKey>('rate')
  const [sortAsc, setSortAsc] = useState(false)
  const [anomalies, setAnomalies] = useState<AnomalyResult[]>([])
  const [slos, setSlos] = useState<SLOResult[]>([])
  const [heatmapData, setHeatmapData] = useState<LatencyHeatmapData>({ bounds: [], buckets: [] })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const services = useMemo(() => [...new Set(metrics.map((metric) => metric.service))], [metrics])
  const effectiveSelectedService = selectedService || services[0] || ''

  const fetchMetrics = useCallback(() => {
    setError(null)
    Promise.all([
      api.getMetrics(),
      api.getAnomalies(),
      api.getSLOs(),
      api.getHeatmap(effectiveSelectedService || undefined),
    ]).then(([metricsRes, anomaliesRes, slosRes, heatmapRes]) => {
      setMetrics(metricsRes.metrics)
      setAnomalies(anomaliesRes.anomalies?.filter((anomaly) => anomaly.isOutlier) ?? [])
      setSlos(slosRes.slos ?? [])
      setHeatmapData(heatmapRes.latency)
    })
      .catch((err: unknown) => setError(getErrorMessage(err, 'Failed to load metrics.')))
      .finally(() => setLoading(false))
  }, [effectiveSelectedService])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      fetchMetrics()
    }, 0)
    const iv = setInterval(fetchMetrics, 5000)
    return () => {
      window.clearTimeout(timer)
      clearInterval(iv)
    }
  }, [fetchMetrics])

  useSSE('/sse/metrics', fetchMetrics)

  const svcMetrics = metrics.filter((metric) => metric.service === effectiveSelectedService)
  const sorted = [...svcMetrics].sort((a, b) => {
    const va = a[sortKey] as number | string
    const vb = b[sortKey] as number | string
    if (typeof va === 'string' && typeof vb === 'string') {
      return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va)
    }
    return sortAsc ? (va as number) - (vb as number) : (vb as number) - (va as number)
  })

  const chartData = sorted.map((metric) => ({
    name: metric.operation.length > 20 ? metric.operation.slice(0, 20) + '\u2026' : metric.operation,
    rate: metric.rate,
    errorRate: metric.errorRate * 100,
    p50: metric.p50Ms,
    p95: metric.p95Ms,
    p99: metric.p99Ms,
  }))

  const serviceSummary = {
    totalRate: svcMetrics.reduce((sum, metric) => sum + metric.rate, 0),
    worstP99: svcMetrics.reduce((max, metric) => Math.max(max, metric.p99Ms), 0),
    highestErrorRate: svcMetrics.reduce((max, metric) => Math.max(max, metric.errorRate), 0),
  }

  const thClass = (key: SortKey) =>
    `text-right p-2 cursor-pointer select-none hover:text-foreground ${sortKey === key ? 'text-foreground font-bold' : 'text-muted-foreground'}`

  const sortIndicator = (key: SortKey) =>
    sortKey === key ? (sortAsc ? ' ↑' : ' ↓') : ''

  const handleSort = (key: SortKey) => {
    if (key === sortKey) setSortAsc((asc) => !asc)
    else {
      setSortKey(key)
      setSortAsc(false)
    }
  }

  if (loading && metrics.length === 0 && !error) {
    return <PageState title="Loading metrics" description="Fetching RED metrics, anomalies, SLOs, and latency heatmaps." />
  }

  if (error && metrics.length === 0) {
    return <PageState title="Unable to load metrics" description={error} actionLabel="Retry" onAction={fetchMetrics} />
  }

  if (chartData.length === 0) {
    return <PageState title="No metrics yet" description="Ingest spans to populate request rate, error rate, and latency trends." />
  }

  return (
    <div className="mx-auto max-w-7xl space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-50px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%),radial-gradient(circle_at_bottom_right,_rgba(250,204,21,0.14),_transparent_38%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.95fr)]">
          <div className="space-y-4">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <Gauge className="h-3.5 w-3.5" />
              RED telemetry
            </div>
            <h1 className="max-w-2xl text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
              Watch rate, saturation signals, and tail latency before the traces tell the same story the hard way.
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
              Slice the metrics surface by service, inspect operations with the worst p99 inflation, and correlate
              anomalies with live heatmaps without leaving the investigation workflow.
            </p>
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-xs font-semibold uppercase tracking-[0.22em] text-muted-foreground">Service focus</span>
              <Select value={effectiveSelectedService} onValueChange={setSelectedService}>
                <SelectTrigger className="w-56 bg-background/70" aria-label="Select service metrics">
                  <SelectValue placeholder="Select service" />
                </SelectTrigger>
                <SelectContent>
                  {services.map((service) => (
                    <SelectItem key={service} value={service}>{service}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Operations</div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{svcMetrics.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Active operations for {effectiveSelectedService || 'the current service'}</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Aggregate rate</div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{serviceSummary.totalRate.toFixed(1)}</div>
              <div className="mt-1 text-xs text-muted-foreground">Requests per second across the selected service</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Worst p99</div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{serviceSummary.worstP99.toFixed(1)}ms</div>
              <div className="mt-1 text-xs text-muted-foreground">Highest current tail latency in the selected slice</div>
            </div>
          </div>
        </div>
      </section>

      {error && (
        <div className="rounded-[24px] border border-amber-300 bg-amber-50/95 px-4 py-3 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
          {error}
        </div>
      )}

      <section className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(300px,0.8fr)]">
        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            <ShieldAlert className="h-3.5 w-3.5" />
            Error budget watch
          </div>
          <div className="mt-4 space-y-3">
            {slos.length > 0 ? slos.map((slo) => (
              <div key={slo.service} className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="font-mono text-sm text-foreground">{slo.service}</div>
                  <span className={`text-xs font-medium ${slo.breached ? 'text-red-600 dark:text-red-300' : 'text-muted-foreground'}`}>
                    {slo.breached ? 'Budget breached' : `${(slo.budgetRemaining * 100).toFixed(1)}% remaining`}
                  </span>
                </div>
                <div className="mt-3 h-3 overflow-hidden rounded-full bg-muted">
                  <div
                    className={`h-full rounded-full transition-all ${slo.breached ? 'bg-red-500' : 'bg-emerald-500'}`}
                    style={{ width: `${Math.max(0, slo.budgetRemaining * 100).toFixed(1)}%` }}
                  />
                </div>
                <div className="mt-2 text-xs text-muted-foreground">
                  Current error rate {(slo.currentErrorRate * 100).toFixed(2)}% against a {(slo.targetErrorRate * 100).toFixed(2)}% target
                </div>
              </div>
            )) : (
              <PageState title="No SLO data" description="Ingest traffic to evaluate error budgets by service." />
            )}
          </div>
        </div>

        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            <AlertTriangle className="h-3.5 w-3.5" />
            Anomaly queue
          </div>
          <div className="mt-4 space-y-3">
            {anomalies.length > 0 ? anomalies.map((anomaly) => (
              <div key={`${anomaly.service}-${anomaly.operation}`} className="rounded-[22px] border border-amber-300 bg-amber-50/90 p-4 dark:bg-amber-950/35">
                <div className="font-mono text-sm font-medium text-amber-900 dark:text-amber-200">{anomaly.service}/{anomaly.operation}</div>
                <div className="mt-2 flex flex-wrap gap-2 text-xs text-amber-800 dark:text-amber-300">
                  <span>P99 {anomaly.p99Ms.toFixed(1)}ms</span>
                  <span>Mean {anomaly.meanMs.toFixed(1)}ms</span>
                  <span>{anomaly.zScore.toFixed(1)}σ above baseline</span>
                </div>
              </div>
            )) : (
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
                No outlier operations are currently breaching the anomaly threshold.
              </div>
            )}
            <div className="rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
              Highest error rate in this service slice is {(serviceSummary.highestErrorRate * 100).toFixed(2)}%.
            </div>
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-3">
        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            <Waves className="h-3.5 w-3.5" />
            Request rate
          </div>
          <div className="mt-3 text-sm text-muted-foreground">See which operations are carrying the service volume right now.</div>
          <div className="mt-4 h-[210px] min-w-0">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                <YAxis />
                <Tooltip />
                <Line type="monotone" dataKey="rate" stroke="#0f766e" dot={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            <ShieldAlert className="h-3.5 w-3.5" />
            Error rate
          </div>
          <div className="mt-3 text-sm text-muted-foreground">Highlight the operations where reliability is degrading before latency spikes dominate.</div>
          <div className="mt-4 h-[210px] min-w-0">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                <YAxis />
                <Tooltip formatter={(v) => typeof v === 'number' ? `${v.toFixed(1)}%` : ''} />
                <Area type="monotone" dataKey="errorRate" stroke="#dc2626" fill="#fca5a5" fillOpacity={0.4} name="Error %" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            <Waypoints className="h-3.5 w-3.5" />
            Tail latency
          </div>
          <div className="mt-3 text-sm text-muted-foreground">Track median behavior against the p95 and p99 edge where users begin to feel the incident.</div>
          <div className="mt-4 h-[210px] min-w-0">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                <YAxis />
                <Tooltip />
                <Legend />
                <Line type="monotone" dataKey="p50" stroke="#059669" dot={false} name="P50" />
                <Line type="monotone" dataKey="p95" stroke="#d97706" dot={false} name="P95" />
                <Line type="monotone" dataKey="p99" stroke="#dc2626" dot={false} name="P99" />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Percentile mix</div>
          <div className="mt-3 text-sm text-muted-foreground">Compare operation-level percentile stacks to spot where long-tail behavior dominates total service time.</div>
          <div className="mt-4 h-[220px] min-w-0">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                <YAxis />
                <Tooltip />
                <Legend />
                <Bar dataKey="p50" stackId="lat" fill="#059669" name="P50ms" />
                <Bar dataKey="p95" stackId="lat" fill="#d97706" name="P95ms" />
                <Bar dataKey="p99" stackId="lat" fill="#dc2626" name="P99ms" />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="min-w-0 rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur sm:p-6">
          <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Latency heatmap</div>
          <div className="mt-3 text-sm text-muted-foreground">Bucket live latency distribution over time to see whether degradation is broad-based or concentrated in a few hot windows.</div>
          <div className="mt-4">
            <LatencyHeatmapChart data={heatmapData} />
          </div>
        </div>
      </section>

      <section className="overflow-hidden rounded-[28px] border border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
        <div className="border-b border-border/70 px-5 py-4">
          <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Operation table</div>
          <div className="mt-1 text-sm text-muted-foreground">Sort the selected service by volume, errors, or any latency percentile.</div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/55">
              <tr>
                <th className="p-2 text-left">
                  <button type="button" className={`select-none hover:text-foreground ${sortKey === 'operation' ? 'text-foreground font-bold' : 'text-muted-foreground'}`} onClick={() => handleSort('operation')}>
                    Operation{sortIndicator('operation')}
                  </button>
                </th>
                <th className="p-2 text-right"><button type="button" className={thClass('rate')} onClick={() => handleSort('rate')}>Rate{sortIndicator('rate')}</button></th>
                <th className="p-2 text-right"><button type="button" className={thClass('errorRate')} onClick={() => handleSort('errorRate')}>Error%{sortIndicator('errorRate')}</button></th>
                <th className="p-2 text-right"><button type="button" className={thClass('p50Ms')} onClick={() => handleSort('p50Ms')}>P50{sortIndicator('p50Ms')}</button></th>
                <th className="p-2 text-right"><button type="button" className={thClass('p95Ms')} onClick={() => handleSort('p95Ms')}>P95{sortIndicator('p95Ms')}</button></th>
                <th className="p-2 text-right"><button type="button" className={thClass('p99Ms')} onClick={() => handleSort('p99Ms')}>P99{sortIndicator('p99Ms')}</button></th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((metric) => (
                <tr key={`${metric.service}-${metric.operation}`} className="border-t border-border/70 hover:bg-accent/40">
                  <td className="p-2 font-mono text-xs">{metric.operation}</td>
                  <td className="p-2 text-right">{metric.rate.toFixed(2)}</td>
                  <td className={`p-2 text-right ${metric.errorRate > 0.1 ? 'text-red-600 font-semibold dark:text-red-300' : ''}`}>
                    {(metric.errorRate * 100).toFixed(1)}%
                  </td>
                  <td className="p-2 text-right">{metric.p50Ms.toFixed(1)}ms</td>
                  <td className="p-2 text-right">{metric.p95Ms.toFixed(1)}ms</td>
                  <td className="p-2 text-right">{metric.p99Ms.toFixed(1)}ms</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
