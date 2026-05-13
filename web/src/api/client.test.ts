import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

const originalFetch = globalThis.fetch

describe('api client', () => {
  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('surfaces structured JSON error messages from the backend', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'query failed' }), {
        status: 500,
        statusText: 'Internal Server Error',
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(api.getMetrics()).rejects.toThrow('HTTP 500: query failed')
  })

  it('times out requests that never resolve', async () => {
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
})
