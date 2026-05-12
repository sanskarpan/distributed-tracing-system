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

export const traceDetailCompareResponse = {
  traceId: 'trace-checkout-fast',
  criticalPath: ['span-root-fast', 'span-auth-fast'],
  services: ['gateway', 'payments', 'postgres'],
  durationMs: 126.3,
  spanCount: 4,
  errorCount: 0,
  parallelGroups: [],
  gaps: [],
  spans: [
    {
      spanId: 'span-root-fast',
      parentSpanId: '',
      traceId: 'trace-checkout-fast',
      name: 'POST /checkout',
      serviceName: 'gateway',
      kind: 2,
      startTimeUnixNano: 2_000_000_000,
      durationMs: 126.3,
      status: { code: 1, message: '' },
      attributes: [{ key: 'http.method', stringValue: 'POST' }],
      events: [],
      links: [],
      depth: 0,
      hasError: false,
    },
    {
      spanId: 'span-auth-fast',
      parentSpanId: 'span-root-fast',
      traceId: 'trace-checkout-fast',
      name: 'authorize payment',
      serviceName: 'payments',
      kind: 3,
      startTimeUnixNano: 2_020_000_000,
      durationMs: 72.2,
      status: { code: 1, message: '' },
      attributes: [{ key: 'payment.provider', stringValue: 'stripe' }],
      events: [],
      links: [],
      depth: 1,
      hasError: false,
    },
    {
      spanId: 'span-db-fast',
      parentSpanId: 'span-auth-fast',
      traceId: 'trace-checkout-fast',
      name: 'INSERT orders',
      serviceName: 'postgres',
      kind: 3,
      startTimeUnixNano: 2_070_000_000,
      durationMs: 31.4,
      status: { code: 1, message: '' },
      attributes: [{ key: 'db.statement', stringValue: 'insert into orders' }],
      events: [],
      links: [],
      depth: 2,
      hasError: false,
    },
    {
      spanId: 'span-log-fast',
      parentSpanId: 'span-root-fast',
      traceId: 'trace-checkout-fast',
      name: 'emit audit event',
      serviceName: 'gateway',
      kind: 4,
      startTimeUnixNano: 2_100_000_000,
      durationMs: 11.1,
      status: { code: 1, message: '' },
      attributes: [{ key: 'messaging.system', stringValue: 'kafka' }],
      events: [],
      links: [],
      depth: 1,
      hasError: false,
    },
  ],
}

export const traceComparisonResponse = {
  durationDeltaMs: -56.1,
  spanCountDelta: -1,
  errorDelta: -1,
  matched: [
    { baseSpanId: 'span-root', compareSpanId: 'span-root-fast', durationDeltaMs: -56.1 },
    { baseSpanId: 'span-auth', compareSpanId: 'span-auth-fast', durationDeltaMs: -38.0 },
    { baseSpanId: 'span-db', compareSpanId: 'span-db-fast', durationDeltaMs: -40.7 },
    { baseSpanId: 'span-log', compareSpanId: 'span-log-fast', durationDeltaMs: -7.1 },
  ],
  onlyInBase: ['span-cache'],
  onlyInCompare: [],
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

export const metricsResponse = {
  metrics: [
    { service: 'gateway', operation: 'POST /checkout', rate: 118.2, errorRate: 0.08, p50Ms: 42.5, p95Ms: 121.4, p99Ms: 182.4 },
    { service: 'gateway', operation: 'GET /catalog', rate: 84.3, errorRate: 0.01, p50Ms: 18.2, p95Ms: 36.6, p99Ms: 64.1 },
    { service: 'payments', operation: 'authorize payment', rate: 63.9, errorRate: 0.12, p50Ms: 55.2, p95Ms: 138.9, p99Ms: 241.7 },
    { service: 'payments', operation: 'capture charge', rate: 31.4, errorRate: 0.03, p50Ms: 24.1, p95Ms: 72.8, p99Ms: 126.2 },
  ],
}

export const anomaliesResponse = {
  anomalies: [
    { service: 'gateway', operation: 'POST /checkout', p99Ms: 182.4, meanMs: 61.2, stddevMs: 24.1, zScore: 5.0, isOutlier: true },
    { service: 'payments', operation: 'authorize payment', p99Ms: 241.7, meanMs: 79.2, stddevMs: 31.8, zScore: 5.1, isOutlier: true },
  ],
}

export const slosResponse = {
  slos: [
    { service: 'gateway', targetErrorRate: 0.01, currentErrorRate: 0.008, budgetRemaining: 0.2, breached: false },
    { service: 'payments', targetErrorRate: 0.01, currentErrorRate: 0.032, budgetRemaining: -0.12, breached: true },
  ],
}

export const heatmapResponse = {
  resolution: '10s',
  buckets: [],
  latency: {
    bounds: [25, 50, 100, 200],
    buckets: [
      { ts: 1_715_497_200, counts: [12, 9, 4, 1, 0] },
      { ts: 1_715_497_210, counts: [11, 10, 5, 2, 1] },
      { ts: 1_715_497_220, counts: [9, 8, 6, 3, 1] },
    ],
  },
}

export const samplerConfigResponse = {
  type: 'adaptive',
  config: { targetRate: 100, minRate: 0.001, maxRate: 1 },
  stats: {
    sampledTotal: 4200,
    droppedTotal: 1800,
    samplingRate: 0.7,
  },
}
