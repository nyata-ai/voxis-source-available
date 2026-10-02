import { test, expect } from '@playwright/test'

test.describe('Voxis-OSS authenticated routes', () => {
  test('opens the dashboard, library, and standard recorder', async ({ page }) => {
    await page.goto('/dashboard')
    await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()

    await page.goto('/library')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.getByRole('textbox', { name: 'Search audio and transcripts' })).toBeVisible()

    await page.goto('/record')
    await expect(page.getByText('Voxis Source-Available', { exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Cancel' })).toHaveAttribute('href', '/library')
    await expect(page.getByText(/Privilege Recording/i)).toHaveCount(0)
  })

  for (const path of ['/url', '/billing', '/register']) {
    test(`${path} has no OSS route`, async ({ page }) => {
      await page.goto(path)
      await expect(page).toHaveURL(/\/$/)
      await expect(page.getByRole('link', { name: 'Sign in' }).first()).toBeVisible()
    })
  }
})
