import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const originalFetch = globalThis.fetch

describe('api client', () => {
  beforeEach(async () => {
    vi.resetModules()
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('surfaces structured JSON error messages from the backend', async () => {
    const { api } = await import('./client')
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'query failed' }), {
        status: 500,
        statusText: 'Internal Server Error',
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(api.getMetrics()).rejects.toThrow('HTTP 500: query failed')
  })

  it('returns readyz payloads even when the collector is overloaded', async () => {
    const { api } = await import('./client')
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        status: 'overloaded',
        uptimeSec: 42,
        goroutines: 18,
        heapMB: 9,
        queueDepth: 780,
        queueCapacity: 1024,
        queueUsagePct: 0.76,
        queueThreshold: 0.7,
      }), {
        status: 503,
        statusText: 'Service Unavailable',
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(api.getReadyz()).resolves.toMatchObject({
      status: 'overloaded',
      queueDepth: 780,
      queueCapacity: 1024,
    })
  })

  it('times out requests that never resolve', async () => {
    const { api } = await import('./client')
    vi.useFakeTimers()
    globalThis.fetch = vi.fn((_input, init) => {
      const signal = init?.signal as AbortSignal | undefined
      return new Promise<Response>((_resolve, reject) => {
        signal?.addEventListener('abort', () => {
          reject(signal.reason ?? new DOMException('Aborted', 'AbortError'))
        }, { once: true })
      })
    }) as typeof fetch

    const pending = api.getMetrics({ timeoutMs: 5 })
    const assertion = expect(pending).rejects.toThrow('Request timed out after 5ms')
    await vi.advanceTimersByTimeAsync(5)

    await assertion
  })

  it('adds configured auth and tenant headers to JSON requests', async () => {
    vi.stubEnv('VITE_API_TOKEN', 'viewer-token')
    vi.stubEnv('VITE_TENANT_ID', 'tenant-a')
    const { api } = await import('./client')

    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ services: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await api.getServices()

    const [, init] = vi.mocked(globalThis.fetch).mock.calls[0]
    const headers = new Headers(init?.headers)
    expect(headers.get('Authorization')).toBe('Bearer viewer-token')
    expect(headers.get('X-Tenant-ID')).toBe('tenant-a')
  })
})
