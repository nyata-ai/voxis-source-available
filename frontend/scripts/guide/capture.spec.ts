import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page } from '@playwright/test'
import { mockApi } from './mock-api'
import { ACTIVE_TURN_INDEX, TRANSCRIPTION_ID, UTTERANCES } from './fixtures'

const here = path.dirname(fileURLToPath(import.meta.url))
const assets = path.resolve(here, '../../src/assets/guide')

function assetPath(name: string): string { return path.join(assets, `${name}.jpg`) }

async function pinAppearance(page: Page): Promise<void> {
  await page.addInitScript(() => {
    window.localStorage.setItem('voxis-theme', JSON.stringify({ state: { theme: 'light' }, version: 0 }))
    window.localStorage.setItem('voxis-lang-v2', 'en')
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: async () => ({ getTracks: () => [{ stop: () => undefined }] }),
        enumerateDevices: async () => [{ kind: 'audioinput', deviceId: 'synthetic-microphone', label: 'Synthetic microphone' }],
      },
    })
  })
}

async function capture(page: Page, name: string): Promise<void> {
  await page.screenshot({ path: assetPath(name), type: 'jpeg', quality: 85, fullPage: false })
}

test.describe('Voxis-OSS guide screenshots', () => {
  test('captures the dashboard, recorder, library search, reader, and export fixture', async ({ page }) => {
    const unmocked = await mockApi(page)
    await pinAppearance(page)

    await page.goto('/dashboard')
    await expect(page.getByRole('heading', { name: /welcome/i })).toBeVisible()
    await page.getByRole('button', { name: 'Upload' }).first().click()
    await expect(page.getByText('Upload audio')).toBeVisible()
    await capture(page, 'dashboard-capture')

    await page.goto('/record')
    await expect(page.getByText(/ready to record|microphone access denied/i)).toBeVisible()
    await capture(page, 'record')

    await page.goto('/library')
    await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible()
    await page.getByRole('textbox', { name: 'Search audio and transcripts' }).fill('library')
    await expect(page.getByRole('button', { name: 'Load more transcript matches' })).toBeVisible()
    await capture(page, 'library-search')

    await page.goto(`/transcriptions/${TRANSCRIPTION_ID}`)
    await expect(page.getByRole('listitem').first()).toBeVisible()
    await page.getByText(UTTERANCES[ACTIVE_TURN_INDEX].text, { exact: false }).first().click()
    await expect(page.getByTestId('activity-card')).toBeVisible()
    await capture(page, 'activity-failure')
    await capture(page, 'reader')

    await page.locator('[data-testid="briefing-band"]').scrollIntoViewIfNeeded()
    await expect(page.locator('[data-testid="briefing-band"]')).toBeVisible()
    await capture(page, 'briefings')

    expect(unmocked).toEqual([])
  })

  test('keeps ordinary recording and transcript continuation usable on a narrow screen', async ({ page }) => {
    const unmocked = await mockApi(page)
    await pinAppearance(page)
    await page.setViewportSize({ width: 390, height: 844 })

    await page.goto('/record')
    await expect(page.getByText(/ready to record|microphone access denied/i)).toBeVisible()

    await page.goto('/library')
    await page.getByRole('textbox', { name: 'Search audio and transcripts' }).fill('library')
    await expect(page.getByRole('button', { name: 'Load more transcript matches' })).toBeVisible()

    expect(unmocked).toEqual([])
  })
})
