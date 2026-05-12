import { useEffect, useState, useRef, useCallback, useMemo } from 'react'
import { Activity, Filter, SlidersHorizontal, TimerReset } from 'lucide-react'
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
} from 'recharts'
import { PageState } from '@/components/ui/page-state'
import { ChartFrame } from '@/components/ui/chart-frame'
import { getErrorMessage } from '@/lib/errors'

interface ThroughputPoint {
  t: string
  sampled: number
  dropped: number
  rate: number
}

const MAX_HISTORY = 60

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

const strategyDescriptions: Record<string, { title: string; summary: string; caution: string }> = {
  always: {
    title: 'Always sample',
    summary: 'Capture every trace when fidelity matters more than ingestion cost.',
    caution: 'Use only when throughput is low enough that storage and browser rendering can keep up.',
  },
  never: {
    title: 'Never sample',
    summary: 'Drop all traces while preserving the ability to reconfigure quickly.',
    caution: 'Useful as a circuit breaker, but it removes the raw data needed for later diagnosis.',
  },
  probabilistic: {
    title: 'Probabilistic sampling',
    summary: 'Apply a stable percentage-based head-sampling rate across incoming traces.',
    caution: 'Good for steady traffic, but rare failure paths may disappear if the rate is too low.',
  },
  ratelimit: {
    title: 'Rate-limited sampling',
    summary: 'Cap accepted traces per second to hold ingest volume inside a fixed budget.',
    caution: 'Burst traffic can bias which traces survive when the limit is reached.',
  },
  adaptive: {
    title: 'Adaptive sampling',
    summary: 'Continuously adjust the head-sampling rate to steer toward a target trace output rate.',
    caution: 'Tune min/max bounds carefully so the controller can react without thrashing.',
  },
  rules: {
    title: 'Rule-based sampling',
    summary: 'Promote or suppress traces using priority-ordered service and operation rules.',
    caution: 'Rule overlap can become hard to reason about; keep scopes narrow and explicit.',
  },
  tail: {
    title: 'Tail-based sampling',
    summary: 'Buffer traces long enough to keep errors, latency outliers, and other post-hoc signals.',
    caution: 'Tail policies improve relevance but consume memory and increase flush latency.',
  },
}

let nextRuleID = 0
let nextPolicyID = 0

export function SamplerPage() {
  const [config, setConfig] = useState<SamplerConfig | null>(null)
  const [selectedType, setSelectedType] = useState('always')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [probRate, setProbRate] = useState(0.1)
  const [rlTps, setRlTps] = useState(100)
  const [adaptTarget, setAdaptTarget] = useState(100)
  const [adaptMin, setAdaptMin] = useState(0.001)
  const [adaptMax, setAdaptMax] = useState(1.0)
  const [rules, setRules] = useState<RuleDraft[]>([])
  const [tailTimeout, setTailTimeout] = useState(10)
  const [tailPolicies, setTailPolicies] = useState<TailPolicyDraft[]>([])
  const [pendingBody, setPendingBody] = useState<Record<string, unknown> | null>(null)
  const [history, setHistory] = useState<ThroughputPoint[]>([])
  const prevSampled = useRef(0)
  const prevDropped = useRef(0)

  const fetchConfig = useCallback(() => {
    setError(null)
    api.getSampler().then((nextConfig) => {
      setConfig(nextConfig)
      setSelectedType(nextConfig.type)

      const now = new Date().toLocaleTimeString('en', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
      const sampledDelta = nextConfig.stats.sampledTotal - prevSampled.current
      const droppedDelta = nextConfig.stats.droppedTotal - prevDropped.current
      prevSampled.current = nextConfig.stats.sampledTotal
      prevDropped.current = nextConfig.stats.droppedTotal

      setHistory((prev) => {
        const next = [...prev, { t: now, sampled: sampledDelta, dropped: droppedDelta, rate: nextConfig.stats.samplingRate * 100 }]
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

  useSSE('/sse/sampler', fetchConfig)

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
      body.policies = tailPolicies.map((policy) => {
        const dto: Record<string, unknown> = { type: policy.type }
        if (policy.type === 'latency') dto.thresholdMs = policy.thresholdMs
        if (policy.type === 'probabilistic') dto.rate = policy.rate
        return dto
      })
    }
    if (selectedType === 'rules') {
      body.rules = rules.map((rule) => ({
        operationGlob: rule.operationGlob,
        serviceName: rule.service,
        priority: rule.priority,
        sampler: { type: rule.samplerType, rate: rule.samplerRate },
      }))
    }
    return body
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

  const strategy = strategyDescriptions[selectedType] ?? strategyDescriptions.always
  const throughputSummary = useMemo(() => {
    const latest = history.at(-1)
    return {
      sampled: latest?.sampled ?? 0,
      dropped: latest?.dropped ?? 0,
      liveRate: latest?.rate ?? ((config?.stats.samplingRate ?? 0) * 100),
    }
  }, [config?.stats.samplingRate, history])

  if (loading && !config) {
    return <PageState title="Loading sampler" description="Fetching current sampler configuration and throughput history." />
  }

  if (error && !config) {
    return <PageState title="Unable to load sampler" description={error} actionLabel="Retry" onAction={fetchConfig} />
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-50px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%),radial-gradient(circle_at_bottom_right,_rgba(52,211,153,0.12),_transparent_38%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.45fr)_minmax(320px,0.95fr)]">
          <div className="space-y-4">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <SlidersHorizontal className="h-3.5 w-3.5" />
              Sampler control plane
            </div>
            <h1 className="max-w-2xl text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
              Tune trace fidelity against ingest cost without losing the operational context behind each decision.
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
              Switch strategies, preview the payload before applying it, and watch sampled versus dropped throughput
              react live as the collector runs.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Current strategy</div>
              <div className="mt-3 text-2xl font-semibold text-foreground">{strategy.title}</div>
              <div className="mt-1 text-xs text-muted-foreground">{strategy.summary}</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Live sampling rate</div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{throughputSummary.liveRate.toFixed(1)}%</div>
              <div className="mt-1 text-xs text-muted-foreground">Based on accepted versus dropped trace decisions</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Operator caution</div>
              <div className="mt-3 text-sm font-medium text-foreground">{strategy.caution}</div>
            </div>
          </div>
        </div>
      </section>

      {error && (
        <div className="rounded-[24px] border border-amber-300 bg-amber-50/95 px-4 py-3 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
          {error}
        </div>
      )}

      {config && (
        <section className="grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(300px,0.85fr)]">
          <Card className="min-w-0 rounded-[28px] border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
            <CardHeader>
              <CardTitle className="text-sm uppercase tracking-[0.22em] text-muted-foreground">Sampling rate history</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid gap-3 sm:grid-cols-3">
                <div className="rounded-[20px] border border-border/70 bg-background/70 p-3">
                  <div className="text-xs text-muted-foreground">Sampled in latest window</div>
                  <div className="mt-2 text-2xl font-semibold text-foreground">{throughputSummary.sampled}</div>
                </div>
                <div className="rounded-[20px] border border-border/70 bg-background/70 p-3">
                  <div className="text-xs text-muted-foreground">Dropped in latest window</div>
                  <div className="mt-2 text-2xl font-semibold text-foreground">{throughputSummary.dropped}</div>
                </div>
                <div className="rounded-[20px] border border-border/70 bg-background/70 p-3">
                  <div className="text-xs text-muted-foreground">Lifetime decisions</div>
                  <div className="mt-2 text-2xl font-semibold text-foreground">{(config.stats.sampledTotal + config.stats.droppedTotal).toLocaleString()}</div>
                </div>
              </div>
              {history.length > 1 ? (
                <div className="mt-4 h-[180px] min-w-0">
                  <ChartFrame className="h-full">
                    {({ width, height }) => (
                    <LineChart width={width} height={height} data={history}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="t" tick={{ fontSize: 9 }} />
                      <YAxis domain={[0, 100]} />
                      <Tooltip formatter={(v) => typeof v === 'number' ? `${v.toFixed(1)}%` : ''} />
                      <Line type="monotone" dataKey="rate" stroke="#2563eb" dot={false} name="Rate %" />
                    </LineChart>
                    )}
                  </ChartFrame>
                </div>
              ) : (
                <div className="mt-4 rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
                  Waiting for a second sampling snapshot before drawing a history line.
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="rounded-[28px] border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
            <CardHeader>
              <CardTitle className="text-sm uppercase tracking-[0.22em] text-muted-foreground">Current sampler state</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <div className="text-xs text-muted-foreground">Pipeline sampler type</div>
                <div data-testid="current-sampler-type" className="mt-2 font-mono text-lg font-semibold text-foreground">{config.type}</div>
              </div>
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <div className="text-xs text-muted-foreground">Sampler config</div>
                <pre className="mt-2 overflow-x-auto text-xs text-foreground">{JSON.stringify(config.config, null, 2)}</pre>
              </div>
            </CardContent>
          </Card>
        </section>
      )}

      <Card className="rounded-[28px] border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
        <CardHeader>
          <CardTitle className="text-sm uppercase tracking-[0.22em] text-muted-foreground">Change sampler</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="grid gap-4 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
            <div className="space-y-3">
              <Select value={selectedType} onValueChange={(value) => { setSelectedType(value); setPendingBody(null) }}>
                <SelectTrigger aria-label="Select sampler strategy" className="bg-background/70">
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
              <div className="rounded-[22px] border border-border/70 bg-background/70 p-4">
                <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                  <Filter className="h-3.5 w-3.5" />
                  Strategy summary
                </div>
                <div className="mt-3 text-lg font-semibold text-foreground">{strategy.title}</div>
                <div className="mt-2 text-sm text-muted-foreground">{strategy.summary}</div>
                <div className="mt-3 text-xs text-muted-foreground">{strategy.caution}</div>
              </div>
            </div>

            <div className="space-y-4">
              {selectedType === 'probabilistic' && (
                <div className="space-y-2 rounded-[22px] border border-border/70 bg-background/70 p-4">
                  <div className="text-sm font-medium text-foreground">Head sampling rate</div>
                  <div className="text-xs text-muted-foreground">Rate: {(probRate * 100).toFixed(0)}%</div>
                  <Slider value={[probRate * 100]} onValueChange={([v]) => setProbRate((v ?? 10) / 100)} min={0} max={100} step={1} />
                </div>
              )}

              {selectedType === 'ratelimit' && (
                <div className="space-y-2 rounded-[22px] border border-border/70 bg-background/70 p-4">
                  <label className="text-sm font-medium text-foreground">Traces per second</label>
                  <input
                    type="number"
                    className="w-full rounded-xl border border-input bg-background px-3 py-2 text-sm"
                    value={rlTps}
                    min={1}
                    onChange={(e) => setRlTps(Number(e.target.value))}
                  />
                </div>
              )}

              {selectedType === 'adaptive' && (
                <div className="space-y-3 rounded-[22px] border border-border/70 bg-background/70 p-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-foreground">Target traces per second</label>
                    <input
                      type="number"
                      className="w-full rounded-xl border border-input bg-background px-3 py-2 text-sm"
                      value={adaptTarget}
                      min={1}
                      onChange={(e) => setAdaptTarget(Number(e.target.value))}
                    />
                  </div>
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-foreground">Minimum rate {(adaptMin * 100).toFixed(1)}%</div>
                    <Slider value={[adaptMin * 100]} onValueChange={([v]) => setAdaptMin((v ?? 0.1) / 100)} min={0.1} max={100} step={0.1} />
                  </div>
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-foreground">Maximum rate {(adaptMax * 100).toFixed(0)}%</div>
                    <Slider value={[adaptMax * 100]} onValueChange={([v]) => setAdaptMax((v ?? 100) / 100)} min={1} max={100} step={1} />
                  </div>
                </div>
              )}

              {selectedType === 'rules' && (
                <div className="space-y-3 rounded-[22px] border border-border/70 bg-background/70 p-4">
                  <div className="text-sm font-medium text-foreground">Priority-ordered rules</div>
                  {rules.map((rule, i) => (
                    <div key={rule.id} className="rounded-[18px] border border-border/70 bg-card/80 p-3 text-xs">
                      <div className="flex gap-2">
                        <input
                          className="flex-1 rounded-lg border border-input bg-background px-2 py-1"
                          placeholder="Operation glob (e.g. HTTP GET *)"
                          value={rule.operationGlob}
                          onChange={(e) => setRules((prev) => prev.map((entry, index) => index === i ? { ...entry, operationGlob: e.target.value } : entry))}
                        />
                        <input
                          className="w-28 rounded-lg border border-input bg-background px-2 py-1"
                          placeholder="Service"
                          value={rule.service}
                          onChange={(e) => setRules((prev) => prev.map((entry, index) => index === i ? { ...entry, service: e.target.value } : entry))}
                        />
                        <input
                          type="number"
                          className="w-16 rounded-lg border border-input bg-background px-2 py-1"
                          placeholder="Priority"
                          value={rule.priority}
                          onChange={(e) => setRules((prev) => prev.map((entry, index) => index === i ? { ...entry, priority: Number(e.target.value) } : entry))}
                        />
                        <button type="button" className="text-red-600 hover:text-red-800" aria-label="Remove rule" onClick={() => setRules((prev) => prev.filter((_, index) => index !== i))}>✕</button>
                      </div>
                      <div className="mt-2 flex gap-2 items-center">
                        <select
                          className="rounded-lg border border-input bg-background px-2 py-1"
                          value={rule.samplerType}
                          onChange={(e) => setRules((prev) => prev.map((entry, index) => index === i ? { ...entry, samplerType: e.target.value } : entry))}
                        >
                          <option value="always">Always</option>
                          <option value="never">Never</option>
                          <option value="probabilistic">Probabilistic</option>
                        </select>
                        {rule.samplerType === 'probabilistic' && (
                          <input
                            type="number"
                            className="w-24 rounded-lg border border-input bg-background px-2 py-1"
                            placeholder="rate 0-1"
                            step={0.01}
                            min={0}
                            max={1}
                            value={rule.samplerRate}
                            onChange={(e) => setRules((prev) => prev.map((entry, index) => index === i ? { ...entry, samplerRate: Number(e.target.value) } : entry))}
                          />
                        )}
                      </div>
                    </div>
                  ))}
                  <button
                    type="button"
                    className="text-sm text-blue-600 hover:underline"
                    aria-label="Add sampler rule"
                    onClick={() => setRules((prev) => [...prev, { id: ++nextRuleID, operationGlob: '*', service: '', priority: 10, samplerType: 'always', samplerRate: 1 }])}
                  >
                    + Add rule
                  </button>
                </div>
              )}

              {selectedType === 'tail' && (
                <div className="space-y-3 rounded-[22px] border border-border/70 bg-background/70 p-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-foreground">Buffer timeout (seconds)</label>
                    <input
                      type="number"
                      className="w-full rounded-xl border border-input bg-background px-3 py-2 text-sm"
                      value={tailTimeout}
                      min={1}
                      onChange={(e) => setTailTimeout(Number(e.target.value))}
                    />
                  </div>
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-foreground">Policy chain</div>
                    {tailPolicies.map((policy, i) => (
                      <div key={policy.id} className="flex gap-2 items-center rounded-[18px] border border-border/70 bg-card/80 p-3 text-xs">
                        <select
                          className="rounded-lg border border-input bg-background px-2 py-1"
                          value={policy.type}
                          onChange={(e) => setTailPolicies((prev) => prev.map((entry, index) => index === i ? { ...entry, type: e.target.value as TailPolicyDraft['type'] } : entry))}
                        >
                          <option value="error">Error</option>
                          <option value="latency">Latency</option>
                          <option value="probabilistic">Probabilistic</option>
                        </select>
                        {policy.type === 'latency' && (
                          <input
                            type="number"
                            className="w-28 rounded-lg border border-input bg-background px-2 py-1"
                            placeholder="threshold ms"
                            value={policy.thresholdMs}
                            onChange={(e) => setTailPolicies((prev) => prev.map((entry, index) => index === i ? { ...entry, thresholdMs: Number(e.target.value) } : entry))}
                          />
                        )}
                        {policy.type === 'probabilistic' && (
                          <input
                            type="number"
                            className="w-24 rounded-lg border border-input bg-background px-2 py-1"
                            placeholder="rate 0-1"
                            step={0.01}
                            min={0}
                            max={1}
                            value={policy.rate}
                            onChange={(e) => setTailPolicies((prev) => prev.map((entry, index) => index === i ? { ...entry, rate: Number(e.target.value) } : entry))}
                          />
                        )}
                        <button type="button" className="ml-auto text-red-600 hover:text-red-800" aria-label="Remove tail policy" onClick={() => setTailPolicies((prev) => prev.filter((_, index) => index !== i))}>✕</button>
                      </div>
                    ))}
                    <button
                      type="button"
                      className="text-sm text-blue-600 hover:underline"
                      aria-label="Add tail policy"
                      onClick={() => setTailPolicies((prev) => [...prev, { id: ++nextPolicyID, type: 'error', thresholdMs: 500, rate: 0.1 }])}
                    >
                      + Add policy
                    </button>
                  </div>
                </div>
              )}

              {selectedType === 'always' || selectedType === 'never' ? (
                <div className="rounded-[22px] border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground">
                  This strategy does not require additional configuration.
                </div>
              ) : null}
            </div>
          </div>

          <div className="flex flex-wrap gap-3">
            <Button onClick={() => setPendingBody(buildBody())}>
              <Activity className="mr-2 h-4 w-4" />
              Preview change
            </Button>
            <span className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/60 px-3 py-1.5 text-xs text-muted-foreground">
              <TimerReset className="h-3.5 w-3.5" />
              Changes apply immediately to new ingest decisions
            </span>
          </div>

          {pendingBody && (
            <div className="rounded-[24px] border border-border/70 bg-muted/40 p-4">
              <div className="text-sm font-semibold text-foreground">Confirm sampler change</div>
              <div className="mt-2 text-sm text-muted-foreground">
                Switch from <span className="font-mono font-semibold">{config?.type ?? '?'}</span> to{' '}
                <span className="font-mono font-semibold">{String(pendingBody.type)}</span>?
              </div>
              <pre className="mt-3 overflow-x-auto rounded-2xl border border-border/70 bg-background/80 p-3 text-xs text-foreground">
                {JSON.stringify(pendingBody, null, 2)}
              </pre>
              <div className="mt-3 flex gap-2">
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
