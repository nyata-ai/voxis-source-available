import { expect, test, type APIResponse, type Page, type Request } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

const enabled = process.env.VOXIS_OSS_RECORDING_E2E === '1'

type JSONRecord = Record<string, unknown>

async function headersFromBrowser(page: Page): Promise<Record<string, string>> {
  let authorization = ''
  const capture = (request: Request) => {
    if (!request.url().includes('/api/v1/')) return
    authorization ||= request.headers().authorization ?? ''
  }
  page.on('request', capture)
  try {
    await page.goto('/dashboard')
    await expect.poll(() => authorization, { timeout: 15_000 }).not.toBe('')
  } finally {
    page.off('request', capture)
  }
  return { Authorization: authorization }
}

async function json(response: APIResponse): Promise<JSONRecord> {
  expect(response.ok()).toBeTruthy()
  return response.json() as Promise<JSONRecord>
}

test.describe('Voxis-OSS browser recording recovery', () => {
  test.skip(!enabled, 'Set VOXIS_OSS_RECORDING_E2E=1 for the VM browser recovery run.')

  test('recovers a real browser microphone recording after a reload', async ({ page }) => {
    test.setTimeout(300_000)
    page.on('dialog', (dialog) => dialog.accept())

    const headers = await headersFromBrowser(page)

    await page.goto('/record')
    await expect(page.getByRole('heading', { name: 'Ready to record' })).toBeVisible()
    const created = page.waitForResponse((response) =>
      response.url().endsWith('/api/v1/recordings') && response.request().method() === 'POST',
    )
    await page.getByRole('button', { name: 'Start Recording' }).click()
    const createdResponse = await created
    expect(createdResponse.status()).toBe(201)
    const session = await createdResponse.json() as { id: string }

    await expect(page.getByText('Recording', { exact: true })).toBeVisible()
    await page.waitForRequest((request) =>
      request.url().includes(`/api/v1/recordings/${session.id}/chunks`) && request.method() === 'POST',
      { timeout: 45_000 },
    )

    await page.reload()
    await expect(page.getByRole('heading', { name: 'Active recording found' })).toBeVisible()
    await page.getByRole('button', { name: 'Take over' }).click()
    await expect(page.getByRole('heading', { name: 'Recording Interrupted' })).toBeVisible()

    const recovered = page.waitForResponse((response) =>
      response.url().endsWith(`/api/v1/recordings/${session.id}/recover`) && response.request().method() === 'POST',
    )
    await page.getByRole('button', { name: 'Recover' }).click()
    expect((await recovered).status()).toBe(200)
    await expect(page).toHaveURL(/\/library$/)

    let sessionState: JSONRecord = {}
    await expect.poll(async () => {
      sessionState = await json(await page.request.get(`/api/v1/recordings/${session.id}`, { headers }))
      return sessionState.status
    }, { timeout: 180_000, intervals: [1_000, 2_000, 4_000] }).toBe('completed')

    const mediaID = String(sessionState.media_id ?? '')
    expect(mediaID).not.toBe('')
    let media: JSONRecord = {}
    await expect.poll(async () => {
      media = await json(await page.request.get(`/api/v1/media/${mediaID}`, { headers }))
      return `${media.status}:${media.scan_status}`
    }, { timeout: 120_000, intervals: [1_000, 2_000, 4_000] }).toBe('ready:scan_clean')

    const stream = await json(await page.request.get(`/api/v1/media/${mediaID}/stream-url`, { headers }))
    const range = await page.request.get(String(stream.url), { headers: { Range: 'bytes=0-31' } })
    expect(range.status()).toBe(206)
    expect(range.headers()['content-range']).toMatch(/^bytes 0-31\//)
    expect((await range.body()).byteLength).toBe(32)
  })
})
