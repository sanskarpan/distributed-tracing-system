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
  CollectorReadyDTO,
} from '@/types'

const BASE = ''  // proxied by Vite
const DEFAULT_TIMEOUT_MS = 15000
const API_TOKEN = import.meta.env.VITE_API_TOKEN?.trim() ?? ''
const TENANT_ID = import.meta.env.VITE_TENANT_ID?.trim() ?? ''

type APIRequestOptions = RequestInit & {
  timeoutMs?: number
}

type FetchJSONOptions = APIRequestOptions & {
  allowedStatuses?: number[]
}

function withTimeoutSignal(signal: AbortSignal | null | undefined, timeoutMs: number) {
  const controller = new AbortController()
  let abortedByTimeout = false

  const abortFromCaller = () => controller.abort(signal?.reason)
  const abortFromTimeout = () => {
    abortedByTimeout = true
    controller.abort(new DOMException(`Request timed out after ${timeoutMs}ms`, 'TimeoutError'))
  }

  const timeoutId = globalThis.setTimeout(abortFromTimeout, timeoutMs)

  if (signal) {
    if (signal.aborted) {
      abortFromCaller()
    } else {
      signal.addEventListener('abort', abortFromCaller, { once: true })
    }
  }

  return {
    signal: controller.signal,
    wasTimeout: () => abortedByTimeout,
    cleanup: () => {
      globalThis.clearTimeout(timeoutId)
      signal?.removeEventListener('abort', abortFromCaller)
    },
  }
}

async function buildErrorMessage(res: Response): Promise<string> {
  const contentType = res.headers.get('content-type') ?? ''
  try {
    if (contentType.includes('application/json')) {
      const payload = await res.json() as { error?: string; message?: string }
      const detail = payload.error ?? payload.message
      if (detail) {
        return `HTTP ${res.status}: ${detail}`
      }
    } else {
      const text = (await res.text()).trim()
      if (text) {
        return `HTTP ${res.status}: ${text}`
      }
    }
  } catch {
    // Fall back to the status line when the response body is unavailable or malformed.
  }
  return `HTTP ${res.status}: ${res.statusText}`
}

async function fetchJSON<T>(url: string, options?: FetchJSONOptions): Promise<T> {
  const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS
  const allowedStatuses = options?.allowedStatuses ?? []
  const callerSignal = options?.signal
  const requestOptions: RequestInit = { ...(options ?? {}) }
  const headers = new Headers(options?.headers ?? {})
  if (API_TOKEN && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${API_TOKEN}`)
  }
  if (TENANT_ID && !headers.has('X-Tenant-ID')) {
    headers.set('X-Tenant-ID', TENANT_ID)
  }
  requestOptions.headers = headers
  delete (requestOptions as FetchJSONOptions).timeoutMs
  delete (requestOptions as FetchJSONOptions).allowedStatuses
  delete (requestOptions as FetchJSONOptions).signal
  const { signal, cleanup, wasTimeout } = withTimeoutSignal(callerSignal, timeoutMs)

  try {
    const res = await fetch(`${BASE}${url}`, { ...requestOptions, signal })
    if (!res.ok && !allowedStatuses.includes(res.status)) {
      throw new Error(await buildErrorMessage(res))
    }
    return res.json() as Promise<T>
  } catch (error) {
    if (wasTimeout()) {
      throw new Error(`Request timed out after ${timeoutMs}ms`, { cause: error })
    }
    throw error
  } finally {
    cleanup()
  }
}

export function buildSSEURL(path: string): string {
  const url = new URL(`${BASE}${path}`, globalThis.location?.origin ?? 'http://localhost')
  if (API_TOKEN && !url.searchParams.has('apiKey')) {
    url.searchParams.set('apiKey', API_TOKEN)
  }
  if (TENANT_ID && !url.searchParams.has('tenantId')) {
    url.searchParams.set('tenantId', TENANT_ID)
  }
  return `${url.pathname}${url.search}`
}

export const api = {
  async getTraces(
    params: Record<string, string | number | boolean | undefined>,
    options?: APIRequestOptions
  ): Promise<TraceListResponse> {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined) {
        qs.set(k, String(v))
      }
    }
    const query = qs.toString() ? `?${qs.toString()}` : ''
    return fetchJSON<TraceListResponse>(`/api/v1/traces${query}`, options)
  },

  async getTrace(traceId: string, options?: APIRequestOptions): Promise<TraceDetailDTO> {
    return fetchJSON<TraceDetailDTO>(`/api/v1/traces/${traceId}`, options)
  },

  async getServices(options?: APIRequestOptions): Promise<{ services: string[] }> {
    return fetchJSON<{ services: string[] }>('/api/v1/services', options)
  },

  async getOperations(service: string, options?: APIRequestOptions): Promise<{ operations: string[] }> {
    return fetchJSON<{ operations: string[] }>(`/api/v1/operations?service=${encodeURIComponent(service)}`, options)
  },

  async getDependencies(options?: APIRequestOptions): Promise<DependencyGraph> {
    return fetchJSON<DependencyGraph>('/api/v1/dependencies', options)
  },

  async getMetrics(options?: APIRequestOptions): Promise<{ metrics: MetricSnapshotDTO[] }> {
    return fetchJSON<{ metrics: MetricSnapshotDTO[] }>('/api/v1/metrics/red', options)
  },

  async getSampler(options?: APIRequestOptions): Promise<SamplerConfig> {
    return fetchJSON<SamplerConfig>('/api/v1/sampler', options)
  },

  async putSampler(config: unknown, options?: APIRequestOptions): Promise<unknown> {
    return fetchJSON<unknown>('/api/v1/sampler', {
      ...options,
      method: 'PUT',
      headers: { ...(options?.headers ?? {}), 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    })
  },

  async compareTraces(baseId: string, compareId: string, options?: APIRequestOptions): Promise<TraceComparisonDTO> {
    return fetchJSON<TraceComparisonDTO>(`/api/v1/traces/compare?base=${baseId}&compare=${compareId}`, options)
  },

  async getConfig(options?: APIRequestOptions): Promise<{ logLinkTemplate?: string }> {
    return fetchJSON<{ logLinkTemplate?: string }>('/api/v1/config', options)
  },

  async getHeatmap(service?: string, options?: APIRequestOptions): Promise<HeatmapResponse> {
    const qs = service ? `?service=${encodeURIComponent(service)}` : ''
    return fetchJSON<HeatmapResponse>(`/api/v1/metrics/heatmap${qs}`, options)
  },

  async getAnomalies(zThreshold?: number, options?: APIRequestOptions): Promise<{ anomalies: AnomalyResult[] }> {
    const qs = zThreshold !== undefined ? `?z=${zThreshold}` : ''
    return fetchJSON<{ anomalies: AnomalyResult[] }>(`/api/v1/metrics/anomalies${qs}`, options)
  },

  async getSLOs(target?: number, options?: APIRequestOptions): Promise<{ slos: SLOResult[] }> {
    const qs = target !== undefined ? `?target=${target}` : ''
    return fetchJSON<{ slos: SLOResult[] }>(`/api/v1/metrics/slo${qs}`, options)
  },

  async getReadyz(options?: APIRequestOptions): Promise<CollectorReadyDTO> {
    return fetchJSON<CollectorReadyDTO>('/readyz', { ...options, allowedStatuses: [503] })
  },
}
