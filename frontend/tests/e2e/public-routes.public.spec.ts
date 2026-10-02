import { test, expect } from '@playwright/test'

test.describe('Public Voxis-OSS routes', () => {
  test('landing explains the installation and links to sign-in and licensing', async ({ page }) => {
    await page.goto('/')
    await expect(page.getByText('Voxis', { exact: true }).first()).toBeVisible()
    await expect(page.getByRole('link', { name: 'Sign in' }).first()).toHaveAttribute('href', '/login')
    await expect(page.getByRole('link', { name: 'Terms and licensing' })).toHaveAttribute('href', '/terms')
    await expect(page.getByRole('link', { name: 'Data flow and security' })).toHaveAttribute('href', '/privacy')
  })

  test('terms and data-flow pages render without a SaaS legal dialog', async ({ page }) => {
    await page.goto('/terms')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    // The product name itself now contains "Source-Available", so match the
    // licensing sentence rather than the bare word.
    await expect(page.getByText('is source-available software', { exact: false })).toBeVisible()
    await page.goto('/privacy')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.getByText('Speechmatics', { exact: false }).first()).toBeVisible()
  })

  test('the user guide opens without signing in', async ({ page }) => {
    await page.goto('/guide')
    await expect(page).toHaveURL(/\/guide$/)
    await expect(page.getByRole('navigation', { name: 'User guide' })).toBeVisible()
  })
})
