import http from 'k6/http'
import { check, sleep } from 'k6'

export const options = {
  vus: 12,
  duration: '45s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<800'],
  },
}

const BASE_URL = __ENV.BASE_URL || 'http://localhost:4318'
const BATCH_SIZE = Number(__ENV.BATCH_SIZE || 20)
const SERVICE_NAME = __ENV.SERVICE_NAME || 'loadgen'
const API_KEY = __ENV.API_KEY || ''
const TENANT_ID = __ENV.TENANT_ID || ''

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
    startTimeUnixNano: (startMs * 1_000_000),
    endTimeUnixNano: ((startMs + durationMs) * 1_000_000),
    attributes: [
      { key: 'http.method', stringValue: parentSpanId ? 'POST' : 'GET' },
      { key: 'deployment.environment', stringValue: 'loadtest' },
    ],
    events: [],
    links: [],
    status: hasError ? { code: 2, message: 'synthetic loadtest failure' } : { code: 1, message: '' },
  }
}

export default function () {
  const spans = []
  const startedAt = Date.now()

  for (let i = 0; i < BATCH_SIZE; i += 1) {
    const traceId = hex(16)
    const rootSpanId = hex(8)
    const childSpanId = hex(8)
    const offset = i * 3
    const hasError = (__VU + __ITER + i) % 17 === 0

    spans.push(
      makeSpan(traceId, rootSpanId, '', 'GET /checkout', `${SERVICE_NAME}-gateway`, startedAt + offset, 120, hasError),
      makeSpan(traceId, childSpanId, rootSpanId, 'POST /payments/authorize', `${SERVICE_NAME}-payments`, startedAt + offset + 8, 70, hasError),
    )
  }

  const res = http.post(`${BASE_URL}/api/v1/spans`, JSON.stringify({ spans }), {
    headers: {
      'Content-Type': 'application/json',
      ...(API_KEY ? { Authorization: `Bearer ${API_KEY}` } : {}),
      ...(TENANT_ID ? { 'X-Tenant-ID': TENANT_ID } : {}),
    },
  })

  check(res, {
    'ingest accepted': (response) => response.status === 200 || response.status === 202,
  })

  sleep(0.25)
}
