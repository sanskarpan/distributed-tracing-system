import http from 'k6/http'
import { check, sleep } from 'k6'

export const options = {
  scenarios: {
    ingest: {
      executor: 'constant-vus',
      exec: 'ingest',
      vus: 10,
      duration: '2m',
    },
    query: {
      executor: 'constant-vus',
      exec: 'query',
      vus: 4,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.02'],
    'http_req_duration{kind:ingest}': ['p(95)<900'],
    'http_req_duration{kind:query}': ['p(95)<700'],
  },
}

const BASE_URL = __ENV.BASE_URL || 'http://localhost:4318'
const BATCH_SIZE = Number(__ENV.BATCH_SIZE || 16)
const SERVICE_NAME = __ENV.SERVICE_NAME || 'mixed-load'
const API_KEY = __ENV.API_KEY || ''
const TENANT_ID = __ENV.TENANT_ID || ''

function requestParams(tags) {
  return {
    headers: {
      'Content-Type': 'application/json',
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      ...(TENANT_ID ? { 'X-Tenant-ID': TENANT_ID } : {}),
    },
    tags,
  }
}

function hex(bytes) {
  let out = ''
  for (let i = 0; i < bytes; i += 1) {
    out += Math.floor(Math.random() * 256).toString(16).padStart(2, '0')
  }
  return out
}

function makeSpan(traceId, spanId, parentSpanId, name, serviceName, startMs, durationMs, hasError) {
  return {
    traceId,
    spanId,
    parentSpanId,
    name,
    kind: parentSpanId ? 3 : 2,
    serviceName,
    startTimeUnixNano: startMs * 1_000_000,
    endTimeUnixNano: (startMs + durationMs) * 1_000_000,
    attributes: [
      { key: 'deployment.environment', stringValue: 'loadtest' },
      { key: 'workflow.mode', stringValue: 'mixed' },
    ],
    events: [],
    links: [],
    status: hasError ? { code: 2, message: 'synthetic mixed-load failure' } : { code: 1, message: '' },
  }
}

export function ingest() {
  const spans = []
  const startedAt = Date.now()

  for (let i = 0; i < BATCH_SIZE; i += 1) {
    const traceId = hex(16)
    const rootSpanId = hex(8)
    const childSpanId = hex(8)
    const dbSpanId = hex(8)
    const offset = i * 4
    const hasError = (__VU + __ITER + i) % 19 === 0

    spans.push(
      makeSpan(traceId, rootSpanId, '', 'POST /checkout', `${SERVICE_NAME}-gateway`, startedAt + offset, 140, hasError),
      makeSpan(traceId, childSpanId, rootSpanId, 'POST /payments/authorize', `${SERVICE_NAME}-payments`, startedAt + offset + 10, 88, hasError),
      makeSpan(traceId, dbSpanId, childSpanId, 'INSERT orders', `${SERVICE_NAME}-postgres`, startedAt + offset + 18, 42, hasError),
    )
  }

  const res = http.post(`${BASE_URL}/api/v1/spans`, JSON.stringify({ spans }), requestParams({ kind: 'ingest' }))
  check(res, {
    'mixed ingest accepted': (response) => response.status === 200 || response.status === 202,
  })
  sleep(0.2)
}

export function query() {
  const endpoints = [
    `${BASE_URL}/api/v1/traces?limit=25&sortBy=receivedAt&sortDesc=true`,
    `${BASE_URL}/api/v1/metrics/red`,
    `${BASE_URL}/api/v1/dependencies`,
    `${BASE_URL}/api/v1/services`,
  ]

  const res = http.get(endpoints[__ITER % endpoints.length], requestParams({ kind: 'query' }))
  check(res, {
    'query path healthy': (response) => response.status >= 200 && response.status < 300,
  })
  sleep(0.35)
}
