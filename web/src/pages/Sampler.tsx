import { useEffect, useState, useRef, useCallback } from 'react'
import { api } from '@/api/client'
import { useSSE } from '@/hooks/useSSE'
import type { SamplerConfig } from '@/types'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Slider } from '@/components/ui/slider'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { PageState } from '@/components/ui/page-state'
import { getErrorMessage } from '@/lib/errors'

// ─────────────────── Throughput history ring buffer ───────────────────

interface ThroughputPoint {
  t: string
  sampled: number
  dropped: number
  rate: number
}

const MAX_HISTORY = 60

// ─────────────────── Config form per sampler type ───────────────────

interface RuleDraft {
  id: number
  operationGlob: string
  service: string
  priority: number
  samplerType: string
  samplerRate: number
}

interface TailPolicyDraft {
  id: number
  type: 'error' | 'latency' | 'probabilistic'
  thresholdMs: number
  rate: number
}

let _ruleId = 0
let _policyId = 0

export function SamplerPage() {
  const [config, setConfig] = useState<SamplerConfig | null>(null)
  const [selectedType, setSelectedType] = useState('always')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Type-specific config state
  const [probRate, setProbRate] = useState(0.1)
  const [rlTps, setRlTps] = useState(100)
  const [adaptTarget, setAdaptTarget] = useState(100)
  const [adaptMin, setAdaptMin] = useState(0.001)
  const [adaptMax, setAdaptMax] = useState(1.0)
  const [rules, setRules] = useState<RuleDraft[]>([])
  const [tailTimeout, setTailTimeout] = useState(10)
  const [tailPolicies, setTailPolicies] = useState<TailPolicyDraft[]>([])

  // Confirmation
  const [pendingBody, setPendingBody] = useState<Record<string, unknown> | null>(null)

  // Throughput history
  const [history, setHistory] = useState<ThroughputPoint[]>([])
  const prevSampled = useRef(0)
  const prevDropped = useRef(0)

  const fetchConfig = useCallback(() => {
    setError(null)
    api.getSampler().then(c => {
      setConfig(c)
      setSelectedType(c.type)

      // Update throughput history
      const now = new Date().toLocaleTimeString('en', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
      const sampledDelta = c.stats.sampledTotal - prevSampled.current
      const droppedDelta = c.stats.droppedTotal - prevDropped.current
      prevSampled.current = c.stats.sampledTotal
      prevDropped.current = c.stats.droppedTotal

      setHistory(prev => {
        const next = [...prev, { t: now, sampled: sampledDelta, dropped: droppedDelta, rate: c.stats.samplingRate * 100 }]
        return next.length > MAX_HISTORY ? next.slice(-MAX_HISTORY) : next
      })
    }).catch((err: unknown) => {
      setError(getErrorMessage(err, 'Failed to load sampler configuration.'))
    }).finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(fetchConfig, 0)
    const iv = setInterval(fetchConfig, 3000)
    return () => {
      window.clearTimeout(timer)
      clearInterval(iv)
    }
  }, [fetchConfig])

  // Live SSE updates
  useSSE('/sse/sampler', fetchConfig)

  // Build request body
  const buildBody = (): Record<string, unknown> => {
    const body: Record<string, unknown> = { type: selectedType }
    if (selectedType === 'probabilistic') body.rate = probRate
    if (selectedType === 'ratelimit') body.tracesPerSec = rlTps
    if (selectedType === 'adaptive') {
      body.targetRate = adaptTarget
      body.minRate = adaptMin
      body.maxRate = adaptMax
    }
    if (selectedType === 'tail') {
      body.bufferTimeoutSec = tailTimeout
      body.policies = tailPolicies.map(p => {
        const dto: Record<string, unknown> = { type: p.type }
        if (p.type === 'latency') dto.thresholdMs = p.thresholdMs
        if (p.type === 'probabilistic') dto.rate = p.rate
        return dto
      })
    }
    if (selectedType === 'rules') {
      body.rules = rules.map(r => ({
        operationGlob: r.operationGlob,
        serviceName: r.service,
        priority: r.priority,
        sampler: { type: r.samplerType, rate: r.samplerRate },
      }))
    }
    return body
  }

  const handleApplyClick = () => {
    setPendingBody(buildBody())
  }

  const handleConfirm = async () => {
    if (!pendingBody) return
    try {
      await api.putSampler(pendingBody)
      setPendingBody(null)
      fetchConfig()
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to apply sampler configuration.'))
    }
  }

  if (loading && !config) {
    return <PageState title="Loading sampler" description="Fetching current sampler configuration and throughput history." />
  }

  if (error && !config) {
    return <PageState title="Unable to load sampler" description={error} actionLabel="Retry" onAction={fetchConfig} />
  }

  return (
    <div className="p-4 max-w-3xl mx-auto space-y-4">
      <h1 className="text-xl font-bold">Sampler Configuration</h1>

      {error && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          {error}
        </div>
      )}

      {/* Stats card */}
      {config && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Current: <span className="font-mono">{config.type}</span></CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-4 text-sm">
              <div>
                <div className="text-muted-foreground">Sampled</div>
                <div className="text-2xl font-bold">{config.stats.sampledTotal.toLocaleString()}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Dropped</div>
                <div className="text-2xl font-bold">{config.stats.droppedTotal.toLocaleString()}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Rate</div>
                <div className="text-2xl font-bold">{(config.stats.samplingRate * 100).toFixed(1)}%</div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Throughput chart */}
      {history.length > 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Sampling Rate History (%)</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={120}>
              <LineChart data={history}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="t" tick={{ fontSize: 9 }} />
                <YAxis domain={[0, 100]} />
                <Tooltip formatter={(v) => typeof v === 'number' ? `${v.toFixed(1)}%` : ''} />
                <Line type="monotone" dataKey="rate" stroke="#2563eb" dot={false} name="Rate %" />
              </LineChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      )}

      {/* Config form */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Change Sampler</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Select value={selectedType} onValueChange={v => { setSelectedType(v); setPendingBody(null) }}>
            <SelectTrigger aria-label="Select sampler strategy">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="always">Always Sample</SelectItem>
              <SelectItem value="never">Never Sample</SelectItem>
              <SelectItem value="probabilistic">Probabilistic</SelectItem>
              <SelectItem value="ratelimit">Rate Limit</SelectItem>
              <SelectItem value="adaptive">Adaptive</SelectItem>
              <SelectItem value="rules">Rule-Based</SelectItem>
              <SelectItem value="tail">Tail-Based</SelectItem>
            </SelectContent>
          </Select>

          {selectedType === 'probabilistic' && (
            <div className="space-y-2">
              <div className="text-sm">Rate: {(probRate * 100).toFixed(0)}%</div>
              <Slider value={[probRate * 100]} onValueChange={([v]) => setProbRate((v ?? 10) / 100)} min={0} max={100} step={1} />
            </div>
          )}

          {selectedType === 'ratelimit' && (
            <div className="space-y-2">
              <label className="text-sm text-muted-foreground">Traces per second</label>
              <input
                type="number"
                className="w-full border rounded px-3 py-1 text-sm"
                value={rlTps}
                min={1}
                onChange={e => setRlTps(Number(e.target.value))}
              />
            </div>
          )}

          {selectedType === 'adaptive' && (
            <div className="space-y-3">
              <div className="space-y-1">
                <label className="text-sm text-muted-foreground">Target traces/sec</label>
                <input
                  type="number"
                  className="w-full border rounded px-3 py-1 text-sm"
                  value={adaptTarget}
                  min={1}
                  onChange={e => setAdaptTarget(Number(e.target.value))}
                />
              </div>
              <div className="space-y-1">
                <div className="text-sm">Min rate: {(adaptMin * 100).toFixed(1)}%</div>
                <Slider value={[adaptMin * 100]} onValueChange={([v]) => setAdaptMin((v ?? 0.1) / 100)} min={0.1} max={100} step={0.1} />
              </div>
              <div className="space-y-1">
                <div className="text-sm">Max rate: {(adaptMax * 100).toFixed(0)}%</div>
                <Slider value={[adaptMax * 100]} onValueChange={([v]) => setAdaptMax((v ?? 100) / 100)} min={1} max={100} step={1} />
              </div>
            </div>
          )}

          {selectedType === 'rules' && (
            <div className="space-y-2">
              <div className="text-xs text-muted-foreground">Rules (evaluated by priority, highest first)</div>
              {rules.map((rule, i) => (
                <div key={rule.id} className="border rounded p-2 space-y-1 text-xs">
                  <div className="flex gap-2">
                    <input
                      className="flex-1 border rounded px-2 py-0.5"
                      placeholder="Operation glob (e.g. HTTP GET *)"
                      value={rule.operationGlob}
                      onChange={e => setRules(prev => prev.map((r, j) => j === i ? { ...r, operationGlob: e.target.value } : r))}
                    />
                    <input
                      className="w-28 border rounded px-2 py-0.5"
                      placeholder="Service filter"
                      value={rule.service}
                      onChange={e => setRules(prev => prev.map((r, j) => j === i ? { ...r, service: e.target.value } : r))}
                    />
                    <input
                      type="number"
                      className="w-16 border rounded px-2 py-0.5"
                      placeholder="Priority"
                      value={rule.priority}
                      onChange={e => setRules(prev => prev.map((r, j) => j === i ? { ...r, priority: Number(e.target.value) } : r))}
                    />
                    <button type="button" className="text-red-600 hover:text-red-800" aria-label="Remove rule" onClick={() => setRules(prev => prev.filter((_, j) => j !== i))}>✕</button>
                  </div>
                  <div className="flex gap-2 items-center">
                    <select
                      className="border rounded px-2 py-0.5 text-xs"
                      value={rule.samplerType}
                      onChange={e => setRules(prev => prev.map((r, j) => j === i ? { ...r, samplerType: e.target.value } : r))}
                    >
                      <option value="always">Always</option>
                      <option value="never">Never</option>
                      <option value="probabilistic">Probabilistic</option>
                    </select>
                    {rule.samplerType === 'probabilistic' && (
                      <input
                        type="number"
                        className="w-20 border rounded px-2 py-0.5"
                        placeholder="rate 0-1"
                        step={0.01}
                        min={0}
                        max={1}
                        value={rule.samplerRate}
                        onChange={e => setRules(prev => prev.map((r, j) => j === i ? { ...r, samplerRate: Number(e.target.value) } : r))}
                      />
                    )}
                  </div>
                </div>
              ))}
              <button
                type="button"
                className="text-xs text-blue-600 hover:underline"
                aria-label="Add sampler rule"
                onClick={() => setRules(prev => [...prev, { id: ++_ruleId, operationGlob: '*', service: '', priority: 10, samplerType: 'always', samplerRate: 1 }])}
              >
                + Add rule
              </button>
            </div>
          )}

          {selectedType === 'tail' && (
            <div className="space-y-3">
              <div className="space-y-1">
                <label className="text-sm text-muted-foreground">Buffer timeout (seconds)</label>
                <input
                  type="number"
                  className="w-full border rounded px-3 py-1 text-sm"
                  value={tailTimeout}
                  min={1}
                  onChange={e => setTailTimeout(Number(e.target.value))}
                />
              </div>
              <div className="space-y-2">
                <div className="text-xs text-muted-foreground">Policy chain (evaluated in order)</div>
                {tailPolicies.map((p, i) => (
                  <div key={p.id} className="border rounded p-2 flex gap-2 items-center text-xs">
                    <select
                      className="border rounded px-2 py-0.5"
                      value={p.type}
                      onChange={e => setTailPolicies(prev => prev.map((q, j) => j === i ? { ...q, type: e.target.value as TailPolicyDraft['type'] } : q))}
                    >
                      <option value="error">Error</option>
                      <option value="latency">Latency</option>
                      <option value="probabilistic">Probabilistic</option>
                    </select>
                    {p.type === 'latency' && (
                      <input
                        type="number"
                        className="w-24 border rounded px-2 py-0.5"
                        placeholder="threshold ms"
                        value={p.thresholdMs}
                        onChange={e => setTailPolicies(prev => prev.map((q, j) => j === i ? { ...q, thresholdMs: Number(e.target.value) } : q))}
                      />
                    )}
                    {p.type === 'probabilistic' && (
                      <input
                        type="number"
                        className="w-20 border rounded px-2 py-0.5"
                        placeholder="rate 0-1"
                        step={0.01}
                        min={0}
                        max={1}
                        value={p.rate}
                        onChange={e => setTailPolicies(prev => prev.map((q, j) => j === i ? { ...q, rate: Number(e.target.value) } : q))}
                      />
                    )}
                    <button type="button" className="text-red-600 ml-auto" aria-label="Remove tail policy" onClick={() => setTailPolicies(prev => prev.filter((_, j) => j !== i))}>✕</button>
                  </div>
                ))}
                <button
                  type="button"
                  className="text-xs text-blue-600 hover:underline"
                  aria-label="Add tail policy"
                  onClick={() => setTailPolicies(prev => [...prev, { id: ++_policyId, type: 'error', thresholdMs: 500, rate: 0.1 }])}
                >
                  + Add policy
                </button>
              </div>
            </div>
          )}

          <Button onClick={handleApplyClick}>Apply</Button>

          {/* Confirmation dialog */}
          {pendingBody && (
            <div className="border rounded-lg p-4 bg-muted/50 space-y-3">
              <div className="text-sm font-semibold">Confirm change</div>
              <div className="text-xs text-muted-foreground">
                Switch from <span className="font-mono font-semibold">{config?.type ?? '?'}</span> to{' '}
                <span className="font-mono font-semibold">{String(pendingBody.type)}</span>?
              </div>
              <pre className="text-xs bg-background p-2 rounded border overflow-x-auto">
                {JSON.stringify(pendingBody, null, 2)}
              </pre>
              <div className="flex gap-2">
                <Button size="sm" onClick={() => { void handleConfirm() }}>Confirm</Button>
                <Button size="sm" variant="outline" onClick={() => setPendingBody(null)}>Cancel</Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
