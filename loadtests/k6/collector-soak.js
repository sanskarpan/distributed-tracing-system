import http from 'k6/http'
import { check, sleep } from 'k6'

export const options = {
  stages: [
    { duration: '2m', target: 8 },
    { duration: '8m', target: 8 },
    { duration: '2m', target: 16 },
    { duration: '8m', target: 16 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<1000'],
  },
}

const BASE_URL = __ENV.BASE_URL || 'http://localhost:4318'
const API_KEY = __ENV.API_KEY || ''
const SERVICE_NAME = __ENV.SERVICE_NAME || 'soak'

function headers() {
  return {
    'Content-Type': 'application/json',
    ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
  }
}

function hex(bytes) {
  let out = ''
  for (let i = 0; i < bytes; i += 1) {
    out += Math.floor(Math.random() * 256).toString(16).padStart(2, '0')
  }
  return out
}

function span(traceId, spanId, parentSpanId, serviceName, name, startedAt, durationMs, hasError) {
  return {
    traceId,
    spanId,
    parentSpanId,
    name,
    kind: parentSpanId ? 3 : 2,
    serviceName,
    startTimeUnixNano: startedAt * 1_000_000,
    endTimeUnixNano: (startedAt + durationMs) * 1_000_000,
    attributes: [
      { key: 'deployment.environment', stringValue: 'soak' },
      { key: 'test.window', stringValue: `${__VU}` },
    ],
    events: [],
    links: [],
    status: hasError ? { code: 2, message: 'synthetic soak failure' } : { code: 1, message: '' },
  }
}

export default function () {
  const traceId = hex(16)
  const rootSpanId = hex(8)
  const paymentSpanId = hex(8)
  const dbSpanId = hex(8)
  const now = Date.now()
  const hasError = (__ITER + __VU) % 29 === 0

  const payload = {
    spans: [
      span(traceId, rootSpanId, '', `${SERVICE_NAME}-gateway`, 'GET /orders', now, 160, hasError),
      span(traceId, paymentSpanId, rootSpanId, `${SERVICE_NAME}-payments`, 'POST /payments/capture', now + 12, 96, hasError),
      span(traceId, dbSpanId, paymentSpanId, `${SERVICE_NAME}-postgres`, 'SELECT orders', now + 24, 56, false),
    ],
  }

  const ingest = http.post(`${BASE_URL}/api/v1/spans`, JSON.stringify(payload), { headers: headers() })
  check(ingest, {
    'soak ingest accepted': (response) => response.status === 200 || response.status === 202,
  })

  if (__ITER % 3 === 0) {
    const stats = http.get(`${BASE_URL}/readyz`, { headers: headers() })
    check(stats, {
      'readyz reachable during soak': (response) => response.status === 200 || response.status === 503,
    })
  }

  if (__ITER % 4 === 0) {
    const traces = http.get(`${BASE_URL}/api/v1/traces?limit=10&sortBy=receivedAt&sortDesc=true`, { headers: headers() })
    check(traces, {
      'trace query reachable during soak': (response) => response.status >= 200 && response.status < 300,
    })
  }

  sleep(0.3)
}
