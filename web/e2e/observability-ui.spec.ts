import { expect, test } from '@playwright/test'
import {
  anomaliesResponse,
  dependencyGraphResponse,
  heatmapResponse,
  installMockEventSource,
  metricsResponse,
  samplerConfigResponse,
  slosResponse,
  traceComparisonResponse,
  traceDetailResponse,
  traceDetailCompareResponse,
  tracesResponse,
} from './support/mock-app'

test.beforeEach(async ({ page }) => {
  await installMockEventSource(page)
  let samplerState = structuredClone(samplerConfigResponse)

  await page.route('**/api/v1/traces?**', async (route) => {
    await route.fulfill({ json: tracesResponse })
  })

  await page.route('**/api/v1/traces/trace-checkout', async (route) => {
    await route.fulfill({ json: traceDetailResponse })
  })

  await page.route('**/api/v1/traces/trace-checkout-fast', async (route) => {
    await route.fulfill({ json: traceDetailCompareResponse })
  })

  await page.route('**/api/v1/traces/compare?**', async (route) => {
    await route.fulfill({ json: traceComparisonResponse })
  })

  await page.route('**/api/v1/config', async (route) => {
    await route.fulfill({ json: { logLinkTemplate: 'https://logs.example.com/trace/{traceId}/span/{spanId}' } })
  })

  await page.route('**/api/v1/dependencies', async (route) => {
    await route.fulfill({ json: dependencyGraphResponse })
  })

  await page.route('**/api/v1/services', async (route) => {
    await route.fulfill({ json: { services: ['gateway', 'payments', 'search', 'profile'] } })
  })

  await page.route('**/api/v1/operations?**', async (route) => {
    await route.fulfill({ json: { operations: ['POST /checkout', 'GET /catalog'] } })
  })

  await page.route('**/api/v1/metrics/red', async (route) => {
    await route.fulfill({ json: metricsResponse })
  })

  await page.route('**/api/v1/metrics/anomalies*', async (route) => {
    await route.fulfill({ json: anomaliesResponse })
  })

  await page.route('**/api/v1/metrics/slo*', async (route) => {
    await route.fulfill({ json: slosResponse })
  })

  await page.route('**/api/v1/metrics/heatmap*', async (route) => {
    await route.fulfill({ json: heatmapResponse })
  })

  await page.route('**/api/v1/sampler', async (route) => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON() as Record<string, unknown>
      samplerState = {
        ...samplerState,
        type: String(body.type),
        config: body,
      }
      await route.fulfill({ json: { ok: true, type: body.type } })
      return
    }

    await route.fulfill({ json: samplerState })
  })
})

test('search page opens a trace from the result stream', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'Find the traces worth opening before the incident window moves on.' })).toBeVisible()
  await expect(page.getByText('POST /checkout')).toBeVisible()
  await page.getByText('POST /checkout').first().click()
  await expect(page.getByText('Trace detail')).toBeVisible()
  await expect(page.getByText('POST /checkout across 3 services')).toBeVisible()
})

test('timeline renders recent trace lanes in the browser', async ({ page }) => {
  await page.goto('/timeline')

  await expect(page.getByRole('heading', { name: 'Watch traces accumulate across services as the incident unfolds.' })).toBeVisible()
  await expect(page.locator('.timeline-svg')).toBeVisible()
  await page.waitForSelector('.lanes text')
  await expect(page.locator('.lanes text').filter({ hasText: /^gateway$/ })).toBeVisible()
  await expect(page.getByText('3 traces across 3 service lanes')).toBeVisible()
})

test('trace detail supports switching between waterfall and flame views', async ({ page }) => {
  await page.goto('/trace/trace-checkout')

  await expect(page.locator('.waterfall-svg')).toBeVisible()
  await page.getByRole('button', { name: /Root cause:/i }).click()
  await expect(page.getByText('View Logs ↗')).toBeVisible()
  await page.getByRole('button', { name: 'Close' }).click()
  await page.getByRole('button', { name: 'Flame' }).click()
  await expect(page.locator('.flame-svg')).toBeVisible()
})

test('metrics page shows service health and can switch service focus', async ({ page }) => {
  await page.goto('/metrics')

  await expect(page.getByRole('heading', { name: /Watch rate, saturation signals, and tail latency/i })).toBeVisible()
  await expect(page.getByText('Error budget watch')).toBeVisible()
  await expect(page.getByRole('cell', { name: 'POST /checkout' })).toBeVisible()

  await page.getByLabel('Select service metrics').click()
  await page.getByRole('option', { name: 'payments' }).click()
  await expect(page.getByRole('cell', { name: 'authorize payment' })).toBeVisible()
  await expect(page.getByText('Budget breached')).toBeVisible()
})

test('metrics page keeps core telemetry visible when a secondary endpoint fails', async ({ page }) => {
  await page.route('**/api/v1/metrics/anomalies*', async (route) => {
    await route.fulfill({
      status: 503,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ error: 'anomaly detector unavailable' }),
    })
  })

  await page.goto('/metrics')

  await expect(page.getByRole('heading', { name: /Watch rate, saturation signals, and tail latency/i })).toBeVisible()
  await expect(page.getByRole('cell', { name: 'POST /checkout' })).toBeVisible()
  await expect(page.getByText(/Anomaly queue unavailable: HTTP 503: anomaly detector unavailable/)).toBeVisible()
})

test('service map exposes selected service context', async ({ page }) => {
  await page.goto('/map')

  await expect(page.getByRole('heading', { name: 'Follow cross-service pressure, saturation, and failure propagation.' })).toBeVisible()
  await expect(page.locator('.react-flow')).toBeVisible()
  await page.getByText('payments').first().click()
  await expect(page.getByText('Selected service')).toBeVisible()
  await expect(page.getByText('P99 latency')).toBeVisible()
})

test('compare page renders structural diff for two traces', async ({ page }) => {
  await page.goto('/compare?base=trace-checkout&compare=trace-checkout-fast')

  await expect(page.getByRole('heading', { name: 'Compare two executions on a shared time axis before you chase the wrong regression.' })).toBeVisible()
  await expect(page.getByText('Duration delta')).toBeVisible()
  await expect(page.getByText('Base trace', { exact: true })).toBeVisible()
  await expect(page.getByText('Compare trace', { exact: true })).toBeVisible()
  await expect(page.getByText('Only in base')).toBeVisible()
})

test('sampler page previews and applies a new strategy', async ({ page }) => {
  await page.goto('/sampler')

  await expect(page.getByRole('heading', { name: 'Tune trace fidelity against ingest cost without losing the operational context behind each decision.' })).toBeVisible()
  await expect(page.getByText('Adaptive sampling').first()).toBeVisible()
  await page.getByLabel('Select sampler strategy').click()
  await page.getByRole('option', { name: 'Probabilistic' }).click()
  await page.getByRole('button', { name: 'Preview change' }).click()
  await expect(page.getByText('"type": "probabilistic"')).toBeVisible()
  await page.getByRole('button', { name: 'Confirm' }).click()
  await expect(page.getByTestId('current-sampler-type')).toHaveText('probabilistic')
})

test('sampler page hydrates the draft from the backend config before previewing', async ({ page }) => {
  const samplerState = {
    type: 'probabilistic',
    config: { rate: 0.35 },
    stats: {
      sampledTotal: 4200,
      droppedTotal: 1800,
      samplingRate: 0.35,
    },
  }

  await page.route('**/api/v1/sampler', async (route) => {
    if (route.request().method() === 'PUT') {
      await route.fulfill({ json: { ok: true } })
      return
    }

    await route.fulfill({ json: samplerState })
  })

  await page.goto('/sampler')

  await expect(page.getByTestId('current-sampler-type')).toHaveText('probabilistic')
  await page.getByRole('button', { name: 'Preview change' }).click()
  await expect(page.locator('pre').filter({ hasText: '"type": "probabilistic"' })).toContainText('"rate": 0.35')
})

test('the app error boundary shows a recovery UI', async ({ page }) => {
  await page.goto('/__e2e/error-boundary')

  await expect(page.getByTestId('app-error-boundary')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reload app' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back to traces' })).toBeVisible()
})

test('unknown routes recover back to trace search', async ({ page }) => {
  await page.goto('/missing-route')

  await expect(page.getByText('Page not found')).toBeVisible()
  await page.getByRole('button', { name: 'Back to traces' }).click()
  await expect(page.getByRole('heading', { name: 'Find the traces worth opening before the incident window moves on.' })).toBeVisible()
})
