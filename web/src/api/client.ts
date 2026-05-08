import type {
  TraceListResponse,
  TraceDetailDTO,
  DependencyGraph,
  MetricSnapshotDTO,
  SamplerConfig,
  TraceComparisonDTO,
  AnomalyResult,
  SLOResult,
  HeatmapResponse,
} from '@/types'

const BASE = ''  // proxied by Vite

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${url}`, options)
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export const api = {
  async getTraces(
    params: Record<string, string | number | boolean | undefined>
  ): Promise<TraceListResponse> {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined) {
        qs.set(k, String(v))
      }
    }
    const query = qs.toString() ? `?${qs.toString()}` : ''
    return fetchJSON<TraceListResponse>(`/api/v1/traces${query}`)
  },

  async getTrace(traceId: string): Promise<TraceDetailDTO> {
    return fetchJSON<TraceDetailDTO>(`/api/v1/traces/${traceId}`)
  },

  async getServices(): Promise<{ services: string[] }> {
    return fetchJSON<{ services: string[] }>('/api/v1/services')
  },

  async getOperations(service: string): Promise<{ operations: string[] }> {
    return fetchJSON<{ operations: string[] }>(`/api/v1/operations?service=${encodeURIComponent(service)}`)
  },

  async getDependencies(): Promise<DependencyGraph> {
    return fetchJSON<DependencyGraph>('/api/v1/dependencies')
  },

  async getMetrics(): Promise<{ metrics: MetricSnapshotDTO[] }> {
    return fetchJSON<{ metrics: MetricSnapshotDTO[] }>('/api/v1/metrics/red')
  },

  async getSampler(): Promise<SamplerConfig> {
    return fetchJSON<SamplerConfig>('/api/v1/sampler')
  },

  async putSampler(config: unknown): Promise<unknown> {
    return fetchJSON<unknown>('/api/v1/sampler', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    })
  },

  async compareTraces(baseId: string, compareId: string): Promise<TraceComparisonDTO> {
    return fetchJSON<TraceComparisonDTO>(`/api/v1/traces/compare?base=${baseId}&compare=${compareId}`)
  },

  async getConfig(): Promise<{ logLinkTemplate?: string }> {
    return fetchJSON<{ logLinkTemplate?: string }>('/api/v1/config')
  },

  async getHeatmap(service?: string): Promise<HeatmapResponse> {
    const qs = service ? `?service=${encodeURIComponent(service)}` : ''
    return fetchJSON<HeatmapResponse>(`/api/v1/metrics/heatmap${qs}`)
  },

  async getAnomalies(zThreshold?: number): Promise<{ anomalies: AnomalyResult[] }> {
    const qs = zThreshold !== undefined ? `?z=${zThreshold}` : ''
    return fetchJSON<{ anomalies: AnomalyResult[] }>(`/api/v1/metrics/anomalies${qs}`)
  },

  async getSLOs(target?: number): Promise<{ slos: SLOResult[] }> {
    const qs = target !== undefined ? `?target=${target}` : ''
    return fetchJSON<{ slos: SLOResult[] }>(`/api/v1/metrics/slo${qs}`)
  },
}
