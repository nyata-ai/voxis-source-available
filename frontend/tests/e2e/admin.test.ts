import { test, expect } from '@playwright/test'

test.describe('OSS administration', () => {
  test('opens operational administration without provider controls', async ({ page }) => {
    await page.goto('/admin')
    await expect(page.getByRole('heading', { name: 'Administration' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Pipeline' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Public prompt management' })).toBeVisible()
    await expect(page.getByText(/Gemini|Gladia|billing/i)).toHaveCount(0)
  })
})