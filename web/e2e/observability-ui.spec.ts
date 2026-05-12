import { expect, test } from '@playwright/test'
import {
  dependencyGraphResponse,
  installMockEventSource,
  traceDetailResponse,
  tracesResponse,
} from './support/mock-app'

test.beforeEach(async ({ page }) => {
  await installMockEventSource(page)

  await page.route('**/api/v1/traces?**', async (route) => {
    await route.fulfill({ json: tracesResponse })
  })

  await page.route('**/api/v1/traces/trace-checkout', async (route) => {
    await route.fulfill({ json: traceDetailResponse })
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
})

test('timeline renders recent trace lanes in the browser', async ({ page }) => {
  await page.goto('/timeline')

  await expect(page.getByRole('heading', { name: 'Watch traces accumulate across services as the incident unfolds.' })).toBeVisible()
  await expect(page.locator('.timeline-svg')).toBeVisible()
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

test('service map exposes selected service context', async ({ page }) => {
  await page.goto('/map')

  await expect(page.getByRole('heading', { name: 'Follow cross-service pressure, saturation, and failure propagation.' })).toBeVisible()
  await expect(page.locator('.react-flow')).toBeVisible()
  await page.getByText('payments').first().click()
  await expect(page.getByText('Selected service')).toBeVisible()
  await expect(page.getByText('P99 latency')).toBeVisible()
})

test('the app error boundary shows a recovery UI', async ({ page }) => {
  await page.goto('/__e2e/error-boundary')

  await expect(page.getByTestId('app-error-boundary')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reload app' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back to traces' })).toBeVisible()
})
