import { useEffect, useState, useCallback } from 'react'
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

type SortKey = 'operation' | 'rate' | 'errorRate' | 'p50Ms' | 'p95Ms' | 'p99Ms'

export function MetricsPage() {
  const [metrics, setMetrics] = useState<MetricSnapshotDTO[]>([])
  const [selectedService, setSelectedService] = useState<string>('')
  const [sortKey, setSortKey] = useState<SortKey>('rate')
  const [sortAsc, setSortAsc] = useState(false)
  const [anomalies, setAnomalies] = useState<AnomalyResult[]>([])
  const [slos, setSlos] = useState<SLOResult[]>([])
  const [heatmapData, setHeatmapData] = useState<LatencyHeatmapData>({ bounds: [], buckets: [] })

  const services = [...new Set(metrics.map(m => m.service))]

  const fetchMetrics = useCallback(() => {
    Promise.all([
      api.getMetrics(),
      api.getAnomalies(),
      api.getSLOs(),
      api.getHeatmap(selectedService || undefined),
    ]).then(([metricsRes, anomaliesRes, slosRes, heatmapRes]) => {
      setMetrics(metricsRes.metrics)
      setAnomalies(anomaliesRes.anomalies?.filter(a => a.isOutlier) ?? [])
      setSlos(slosRes.slos ?? [])
      setHeatmapData(heatmapRes.latency)
    }).catch(console.error)
  }, [selectedService])

  useEffect(() => {
    fetchMetrics()
    const iv = setInterval(fetchMetrics, 5000)
    return () => clearInterval(iv)
  }, [fetchMetrics])

  // Live update on any SSE event
  useSSE('/sse/metrics', fetchMetrics)

  useEffect(() => {
    if (!selectedService && services.length > 0) {
      setSelectedService(services[0])
    }
  }, [services, selectedService])

  const svcMetrics = metrics.filter(m => m.service === selectedService)

  const sorted = [...svcMetrics].sort((a, b) => {
    const va = a[sortKey] as number | string
    const vb = b[sortKey] as number | string
    if (typeof va === 'string' && typeof vb === 'string') {
      return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va)
    }
    return sortAsc ? (va as number) - (vb as number) : (vb as number) - (va as number)
  })

  const chartData = sorted.map(m => ({
    name: m.operation.length > 20 ? m.operation.slice(0, 20) + '\u2026' : m.operation,
    rate: m.rate,
    errorRate: m.errorRate * 100,
    p50: m.p50Ms,
    p95: m.p95Ms,
    p99: m.p99Ms,
  }))

  const handleSort = (key: SortKey) => {
    if (key === sortKey) setSortAsc(a => !a)
    else { setSortKey(key); setSortAsc(false) }
  }

  const thClass = (key: SortKey) =>
    `text-right p-2 cursor-pointer select-none hover:text-foreground ${sortKey === key ? 'text-foreground font-bold' : 'text-muted-foreground'}`

  const sortIndicator = (key: SortKey) =>
    sortKey === key ? (sortAsc ? ' ↑' : ' ↓') : ''

  return (
    <div className="p-4 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <h1 className="text-xl font-bold">Metrics</h1>
        <Select value={selectedService} onValueChange={setSelectedService}>
          <SelectTrigger className="w-48">
            <SelectValue placeholder="Select service" />
          </SelectTrigger>
          <SelectContent>
            {services.map(s => (
              <SelectItem key={s} value={s}>{s}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {slos.length > 0 && (
        <div className="border rounded-lg p-4">
          <h3 className="text-sm font-semibold mb-3">Error Budget (SLO: &lt;1% errors)</h3>
          <div className="space-y-2">
            {slos.map((s, i) => (
              <div key={i} className="flex items-center gap-3">
                <span className="text-xs font-mono w-32 shrink-0">{s.service}</span>
                <div className="flex-1 bg-muted rounded-full h-3 overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all ${s.breached ? 'bg-red-500' : 'bg-green-500'}`}
                    style={{ width: `${Math.max(0, s.budgetRemaining * 100).toFixed(1)}%` }}
                  />
                </div>
                <span className={`text-xs w-20 text-right ${s.breached ? 'text-red-600 font-semibold' : 'text-muted-foreground'}`}>
                  {s.breached ? 'BREACHED' : `${(s.budgetRemaining * 100).toFixed(1)}% left`}
                </span>
                <span className="text-xs text-muted-foreground w-20 text-right">
                  {(s.currentErrorRate * 100).toFixed(2)}% err
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {anomalies.length > 0 && (
        <div className="border border-amber-400 bg-amber-50 dark:bg-amber-950/30 rounded-lg p-3">
          <h3 className="text-sm font-semibold text-amber-800 dark:text-amber-300 mb-2">
            Latency Anomalies ({anomalies.length})
          </h3>
          <div className="space-y-1">
            {anomalies.map((a, i) => (
              <div key={i} className="text-xs text-amber-700 dark:text-amber-400 flex gap-3">
                <span className="font-mono font-semibold">{a.service}/{a.operation}</span>
                <span>P99: {a.p99Ms.toFixed(1)}ms</span>
                <span>z-score: {a.zScore.toFixed(1)}σ</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {chartData.length === 0 ? (
        <p className="text-muted-foreground text-center py-8">
          No metrics yet. Ingest spans to see metrics here.
        </p>
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {/* Rate chart */}
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold mb-2">Request Rate (req/s)</h3>
              <ResponsiveContainer width="100%" height={180}>
                <LineChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                  <YAxis />
                  <Tooltip />
                  <Line type="monotone" dataKey="rate" stroke="#2563eb" dot={false} />
                </LineChart>
              </ResponsiveContainer>
            </div>

            {/* Error rate area chart */}
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold mb-2">Error Rate (%)</h3>
              <ResponsiveContainer width="100%" height={180}>
                <AreaChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                  <YAxis />
                  <Tooltip formatter={(v) => typeof v === 'number' ? `${v.toFixed(1)}%` : ''} />
                  <Area
                    type="monotone"
                    dataKey="errorRate"
                    stroke="#dc2626"
                    fill="#fca5a5"
                    fillOpacity={0.4}
                    name="Error %"
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>

            {/* Latency chart */}
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold mb-2">Latency Percentiles (ms)</h3>
              <ResponsiveContainer width="100%" height={180}>
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

          {/* Heatmap (latency distribution per operation) */}
          {svcMetrics.length > 0 && (
            <div className="border rounded-lg p-4">
              <h3 className="text-sm font-semibold mb-2">Latency Heatmap (P50/P95/P99 per operation)</h3>
              <ResponsiveContainer width="100%" height={120}>
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
          )}

          {/* 2D Latency Heatmap (time × latency bucket) */}
          <div className="border rounded-lg p-4">
            <h3 className="text-sm font-semibold mb-2">Real-time Latency Heatmap (time × latency band)</h3>
            <LatencyHeatmapChart data={heatmapData} />
          </div>

          {/* Sortable operations table */}
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-muted">
                <tr>
                  <th
                    className={`text-left p-2 cursor-pointer select-none hover:text-foreground ${sortKey === 'operation' ? 'text-foreground font-bold' : 'text-muted-foreground'}`}
                    onClick={() => handleSort('operation')}
                  >
                    Operation{sortIndicator('operation')}
                  </th>
                  <th className={thClass('rate')} onClick={() => handleSort('rate')}>Rate{sortIndicator('rate')}</th>
                  <th className={thClass('errorRate')} onClick={() => handleSort('errorRate')}>Error%{sortIndicator('errorRate')}</th>
                  <th className={thClass('p50Ms')} onClick={() => handleSort('p50Ms')}>P50{sortIndicator('p50Ms')}</th>
                  <th className={thClass('p95Ms')} onClick={() => handleSort('p95Ms')}>P95{sortIndicator('p95Ms')}</th>
                  <th className={thClass('p99Ms')} onClick={() => handleSort('p99Ms')}>P99{sortIndicator('p99Ms')}</th>
                </tr>
              </thead>
              <tbody>
                {sorted.map((m, i) => (
                  <tr key={i} className="border-t hover:bg-accent">
                    <td className="p-2 font-mono text-xs">{m.operation}</td>
                    <td className="p-2 text-right">{m.rate.toFixed(2)}</td>
                    <td className={`p-2 text-right ${m.errorRate > 0.1 ? 'text-red-600 font-semibold' : ''}`}>
                      {(m.errorRate * 100).toFixed(1)}%
                    </td>
                    <td className="p-2 text-right">{m.p50Ms.toFixed(1)}ms</td>
                    <td className="p-2 text-right">{m.p95Ms.toFixed(1)}ms</td>
                    <td className="p-2 text-right">{m.p99Ms.toFixed(1)}ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
