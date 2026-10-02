import { readFile } from 'node:fs/promises'
import { expect, test, type APIResponse, type Page, type Request } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

const enabled = process.env.VOXIS_OSS_SCAN_E2E === '1'

type JSONRecord = Record<string, unknown>

function required(name: string): string {
  const value = process.env[name]
  if (!value) throw new Error(`${name} is required when VOXIS_OSS_SCAN_E2E=1.`)
  return value
}

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

async function upload(page: Page, headers: Record<string, string>, name: string, mimeType: string, buffer: Buffer): Promise<string> {
  const created = await page.request.post('/api/v1/media/upload', {
    headers,
    multipart: { file: { name, mimeType, buffer } },
  })
  expect(created.status()).toBe(201)
  return String((await json(created)).id)
}

async function waitForScan(
  page: Page,
  headers: Record<string, string>,
  mediaID: string,
  expectedScan: string,
): Promise<JSONRecord> {
  let media: JSONRecord = {}
  await expect.poll(async () => {
    media = await json(await page.request.get(`/api/v1/media/${mediaID}`, { headers }))
    const state = String(media.scan_status ?? '')
    if (state === 'scan_error') throw new Error('scanner returned scan_error; incomplete scanning is not clean')
    return state
  }, { timeout: 120_000, intervals: [1_000, 2_000, 4_000] }).toBe(expectedScan)
  return media
}

test.describe('Voxis-OSS malware scan gate', () => {
  test.skip(!enabled, 'Set VOXIS_OSS_SCAN_E2E=1 only for the disposable VM acceptance run.')

  test('streams clean media, accepts the Windows AAC alias, and quarantines a scanner-positive control', async ({ page }) => {
    test.setTimeout(420_000)
    const wav = await readFile(required('VOXIS_OSS_ACCEPTANCE_WAV_PATH'))
    const aac = await readFile(required('VOXIS_OSS_ACCEPTANCE_AAC_PATH'))
    const scannerPositive = await readFile(required('VOXIS_OSS_ACCEPTANCE_SCANNER_POSITIVE_WAV_PATH'))
    const headers = await headersFromBrowser(page)
    const mediaIDs: string[] = []

    try {
      const cleanID = await upload(page, headers, 'synthetic-meeting.wav', 'audio/wav', wav)
      mediaIDs.push(cleanID)
      const clean = await waitForScan(page, headers, cleanID, 'scan_clean')
      expect(clean.status).toBe('ready')

      const stream = await json(await page.request.get(`/api/v1/media/${cleanID}/stream-url`, { headers }))
      const range = await page.request.get(String(stream.url), { headers: { Range: 'bytes=0-31' } })
      expect(range.status()).toBe(206)
      expect((await range.body()).byteLength).toBe(32)

      const aliasID = await upload(page, headers, 'synthetic-meeting.aac', 'audio/vnd.dlna.adts', aac)
      mediaIDs.push(aliasID)
      const alias = await waitForScan(page, headers, aliasID, 'scan_clean')
      expect(alias.content_type).toBe('audio/aac')

      const infectedID = await upload(page, headers, 'scanner-positive-control.wav', 'audio/wav', scannerPositive)
      mediaIDs.push(infectedID)
      const infected = await waitForScan(page, headers, infectedID, 'scan_infected')
      expect(infected.status).toBe('failed')

      const blocked = await page.request.get(`/api/v1/media/${infectedID}/stream-url`, { headers })
      expect(blocked.status()).toBe(403)
      expect((await blocked.json() as JSONRecord).error).toBe('scan_failed')
    } finally {
      await Promise.all(mediaIDs.map(async (id) => {
        await page.request.delete(`/api/v1/media/${id}`, { headers })
      }))
    }
  })
})
