import type { Page } from '@playwright/test'

export async function installMockEventSource(page: Page) {
  await page.addInitScript(() => {
    class MockEventSource {
      onmessage: ((event: MessageEvent) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor() {}

      addEventListener() {}

      removeEventListener() {}

      close() {}
    }

    Object.defineProperty(window, 'EventSource', {
      configurable: true,
      writable: true,
      value: MockEventSource,
    })
  })
}

export const tracesResponse = {
  traces: [
    {
      traceId: 'trace-checkout',
      rootService: 'gateway',
      rootOp: 'POST /checkout',
      durationMs: 182.4,
      spanCount: 5,
      services: ['gateway', 'payments', 'postgres'],
      hasError: true,
      receivedAt: '2026-05-12T06:00:00.000Z',
    },
    {
      traceId: 'trace-search',
      rootService: 'search',
      rootOp: 'GET /catalog',
      durationMs: 64.1,
      spanCount: 3,
      services: ['search', 'redis'],
      hasError: false,
      receivedAt: '2026-05-12T06:00:04.000Z',
    },
    {
      traceId: 'trace-profile',
      rootService: 'profile',
      rootOp: 'GET /profile',
      durationMs: 98.8,
      spanCount: 4,
      services: ['profile', 'postgres'],
      hasError: false,
      receivedAt: '2026-05-12T06:00:09.000Z',
    },
  ],
  total: 3,
  hasMore: false,
}

export const traceDetailResponse = {
  traceId: 'trace-checkout',
  criticalPath: ['span-root', 'span-db'],
  services: ['gateway', 'payments', 'postgres'],
  durationMs: 182.4,
  spanCount: 5,
  errorCount: 1,
  parallelGroups: [],
  gaps: [],
  spans: [
    {
      spanId: 'span-root',
      parentSpanId: '',
      traceId: 'trace-checkout',
      name: 'POST /checkout',
      serviceName: 'gateway',
      kind: 2,
      startTimeUnixNano: 1_000_000_000,
      durationMs: 182.4,
      status: { code: 2, message: 'payment authorization failed' },
      attributes: [{ key: 'http.method', stringValue: 'POST' }],
      events: [],
      links: [],
      depth: 0,
      hasError: true,
    },
    {
      spanId: 'span-auth',
      parentSpanId: 'span-root',
      traceId: 'trace-checkout',
      name: 'authorize payment',
      serviceName: 'payments',
      kind: 3,
      startTimeUnixNano: 1_020_000_000,
      durationMs: 110.2,
      status: { code: 2, message: 'card declined' },
      attributes: [{ key: 'payment.provider', stringValue: 'stripe' }],
      events: [{ timeUnixNano: 1_040_000_000, name: 'retry', attributes: [] }],
      links: [],
      depth: 1,
      hasError: true,
    },
    {
      spanId: 'span-db',
      parentSpanId: 'span-auth',
      traceId: 'trace-checkout',
      name: 'INSERT orders',
      serviceName: 'postgres',
      kind: 3,
      startTimeUnixNano: 1_090_000_000,
      durationMs: 72.1,
      status: { code: 2, message: 'unique constraint violation' },
      attributes: [{ key: 'db.statement', stringValue: 'insert into orders' }],
      events: [],
      links: [],
      depth: 2,
      hasError: true,
    },
    {
      spanId: 'span-cache',
      parentSpanId: 'span-root',
      traceId: 'trace-checkout',
      name: 'refresh basket cache',
      serviceName: 'gateway',
      kind: 1,
      startTimeUnixNano: 1_025_000_000,
      durationMs: 28.7,
      status: { code: 1, message: '' },
      attributes: [{ key: 'cache.hit', boolValue: false }],
      events: [],
      links: [],
      depth: 1,
      hasError: false,
    },
    {
      spanId: 'span-log',
      parentSpanId: 'span-root',
      traceId: 'trace-checkout',
      name: 'emit audit event',
      serviceName: 'gateway',
      kind: 4,
      startTimeUnixNano: 1_130_000_000,
      durationMs: 18.2,
      status: { code: 1, message: '' },
      attributes: [{ key: 'messaging.system', stringValue: 'kafka' }],
      events: [],
      links: [],
      depth: 1,
      hasError: false,
    },
  ],
}

export const dependencyGraphResponse = {
  services: [
    { name: 'gateway', spanCount: 5210, errorRate: 0.06, p99Ms: 192, reqPerSec: 118.2 },
    { name: 'payments', spanCount: 3180, errorRate: 0.13, p99Ms: 241, reqPerSec: 84.1 },
    { name: 'postgres', spanCount: 4470, errorRate: 0.03, p99Ms: 138, reqPerSec: 97.5 },
  ],
  edges: [
    { caller: 'gateway', callee: 'payments', count: 1510, errorCount: 82, p99Ms: 214 },
    { caller: 'payments', callee: 'postgres', count: 980, errorCount: 33, p99Ms: 141 },
    { caller: 'gateway', callee: 'postgres', count: 420, errorCount: 9, p99Ms: 94 },
  ],
}
