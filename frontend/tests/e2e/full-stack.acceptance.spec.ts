import { expect, test, type APIResponse, type Page, type Request } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

const enabled = process.env.VOXIS_OSS_FULL_STACK_E2E === '1'

type Fixture = {
  mediaID: string
  transcriptionID: string
  oversizedTranscriptionID: string
  lateSegmentID: string
}

type JSONRecord = Record<string, unknown>
type AuthHeaders = () => Record<string, string>

function required(name: string): string {
  const value = process.env[name]
  if (!value) throw new Error(`${name} is required when VOXIS_OSS_FULL_STACK_E2E=1.`)
  return value
}

function fixture(): Fixture {
  return {
    mediaID: required('VOXIS_OSS_ACCEPTANCE_MEDIA_ID'),
    transcriptionID: required('VOXIS_OSS_ACCEPTANCE_TRANSCRIPTION_ID'),
    oversizedTranscriptionID: required('VOXIS_OSS_ACCEPTANCE_OVERSIZED_TRANSCRIPTION_ID'),
    lateSegmentID: required('VOXIS_OSS_ACCEPTANCE_LATE_SEGMENT_ID'),
  }
}

async function headersFromBrowser(page: Page): Promise<AuthHeaders> {
  let authorization = ''
  const capture = (request: Request) => {
    if (!request.url().includes('/api/v1/')) return
    const refreshedAuthorization = request.headers().authorization ?? ''
    if (refreshedAuthorization) authorization = refreshedAuthorization
  }
  page.on('request', capture)
  await page.goto('/dashboard')
  await expect.poll(() => authorization, { timeout: 15_000 }).not.toBe('')
  // Keep listening while the dashboard's activity heartbeat refreshes the
  // browser token. Summaries may take longer than one access-token lifetime.
  return () => ({ Authorization: authorization })
}

async function json(response: APIResponse): Promise<JSONRecord> {
  expect(response.ok()).toBeTruthy()
  return response.json() as Promise<JSONRecord>
}

async function waitForSummary(
  page: Page,
  headers: AuthHeaders,
  summaryID: string,
  expected: 'completed' | 'failed',
): Promise<JSONRecord> {
  let latest: JSONRecord = {}
  await expect.poll(async () => {
    const response = await page.request.get(`/api/v1/summaries/${summaryID}`, { headers: headers() })
    latest = await json(response)
    return latest.status
  }, { timeout: 600_000, intervals: [1_000, 2_000, 4_000, 8_000] }).toBe(expected)
  return latest
}

async function mcpRequest(page: Page, key: string, id: number, method: string, params: unknown): Promise<JSONRecord> {
  const response = await page.request.post('/mcp', {
    headers: {
      Authorization: `Bearer ${key}`,
      Accept: 'application/json, text/event-stream',
      'Content-Type': 'application/json',
    },
    data: { jsonrpc: '2.0', id, method, params },
  })
  expect(response.status()).toBe(200)
  const body = await response.json() as JSONRecord
  expect(body.error).toBeFalsy()
  return body
}

test.use({ trace: 'off', screenshot: 'off', video: 'off' })

test.describe('Voxis-OSS synthetic full-stack acceptance', () => {
  test.skip(!enabled, 'Set VOXIS_OSS_FULL_STACK_E2E=1 only for the disposable VM acceptance run.')

  test('reads, ranges, edits, searches, exports, and inspects the synthetic transcript', async ({ page }) => {
    const { mediaID, transcriptionID } = fixture()
    const headers = await headersFromBrowser(page)
    const original = await json(await page.request.get(`/api/v1/media/${mediaID}`, { headers: headers() }))
    const acceptanceTitle = `OSS acceptance ${Date.now()}`

    try {
      const transcription = await json(
        await page.request.get(`/api/v1/transcriptions/${transcriptionID}`, { headers: headers() }),
      )
      expect(transcription.status).toBe('completed')
      expect(String(transcription.full_transcript ?? '')).toContain('limited pilot')

      await page.goto(`/transcriptions/${transcriptionID}`)
      await expect(page.locator('audio')).toHaveCount(1)

      const stream = await json(
        await page.request.get(`/api/v1/media/${mediaID}/stream-url`, { headers: headers() }),
      )
      const range = await page.request.get(String(stream.url), { headers: { Range: 'bytes=0-31' } })
      expect(range.status()).toBe(206)
      expect(range.headers()['content-range']).toMatch(/^bytes 0-31\//)
      expect((await range.body()).byteLength).toBe(32)

      const updated = await page.request.patch(`/api/v1/media/${mediaID}`, {
        headers: headers(),
        data: { title: acceptanceTitle },
      })
      expect(updated.status()).toBe(200)
      const search = await json(
        await page.request.get(`/api/v1/search?search=${encodeURIComponent(acceptanceTitle)}`, { headers: headers() }),
      )
      const matches = Array.isArray(search.items) ? search.items : []
      expect(matches.some((item) => (item as JSONRecord).id === mediaID)).toBeTruthy()

      for (const format of ['json', 'pdf', 'docx']) {
        const exported = await page.request.get(`/api/v1/transcriptions/${transcriptionID}/export?format=${format}`, { headers: headers() })
        expect(exported.status()).toBe(200)
        const bytes = await exported.body()
        expect(bytes.byteLength).toBeGreaterThan(8)
        if (format === 'json') expect(JSON.parse(bytes.toString())).toHaveProperty('full_transcript')
        if (format === 'pdf') expect(bytes.subarray(0, 4).toString()).toBe('%PDF')
        if (format === 'docx') expect(bytes.subarray(0, 2).toString()).toBe('PK')
      }
    } finally {
      await page.request.patch(`/api/v1/media/${mediaID}`, {
        headers: headers(),
        data: { title: original.title ?? '' },
      })
    }
  })

  test('persists standard and anchored summaries and rejects the oversized source', async ({ page }) => {
    // Two CPU-only model runs each get a bounded 10-minute poll budget, plus
    // setup and persisted-result checks. This is intentionally separate from
    // the <=120s audio provider fixture limit.
    test.setTimeout(1_300_000)
    const { transcriptionID, oversizedTranscriptionID, lateSegmentID } = fixture()
    const headers = await headersFromBrowser(page)

    const standard = await json(await page.request.post('/api/v1/summaries', {
      headers: headers(),
      data: { transcription_id: transcriptionID, summary_type: 'general', high_stakes: false },
    }))
    expect(standard.status).toBe('pending')
    const completedStandard = await waitForSummary(page, headers, String(standard.id), 'completed')
    const standardProse = String(completedStandard.content ?? '')
    expect(standardProse).toMatch(/limited pilot/i)
    expect(standardProse).toMatch(/retention schedule/i)
    expect(standardProse).toMatch(/Nia/i)
    expect(standardProse).toMatch(/2026[-/ ]?10[-/ ]?23|23(?:st|nd|rd|th)?\s+(?:October|Oct)\s+2026|(?:October|Oct)\s+23(?:st|nd|rd|th)?,?\s+2026/i)
    expect(standardProse).toMatch(/Elena/i)
    expect(standardProse).toMatch(/Q4/i)
    expect(standardProse).toMatch(/uncertain|uncertainty|not confirmed|pending/i)

    const anchored = await json(await page.request.post('/api/v1/summaries', {
      headers: headers(),
      data: { transcription_id: transcriptionID, summary_type: 'key_points', high_stakes: true },
    }))
    expect(anchored.high_stakes).toBe(true)
    const completedAnchored = await waitForSummary(page, headers, String(anchored.id), 'completed')
    const anchoredProse = String(completedAnchored.content ?? '')
    expect(anchoredProse).toMatch(/limited pilot/i)
    expect(anchoredProse).toMatch(/retention schedule/i)
    expect(anchoredProse).toMatch(/Nia/i)
    expect(anchoredProse).toMatch(/2026[-/ ]?10[-/ ]?23|23(?:st|nd|rd|th)?\s+(?:October|Oct)\s+2026|(?:October|Oct)\s+23(?:st|nd|rd|th)?,?\s+2026/i)
    expect(completedAnchored.structured_content).toBeTruthy()
    const citations = Array.isArray(completedAnchored.citations) ? completedAnchored.citations : []
    expect(citations.some((citation) => (citation as JSONRecord).id === lateSegmentID)).toBeTruthy()

    const oversized = await json(await page.request.post('/api/v1/summaries', {
      headers: headers(),
      data: { transcription_id: oversizedTranscriptionID, summary_type: 'general', high_stakes: false },
    }))
    const failed = await waitForSummary(page, headers, String(oversized.id), 'failed')
    expect(String(failed.error_message ?? '')).toMatch(/summary source exceeds supported input size/i)
  })

  test('creates and revokes a browser API key, calls MCP, and reads operational stats', async ({ page }) => {
    const { mediaID } = fixture()
    const headers = await headersFromBrowser(page)
    const name = `OSS acceptance ${Date.now()}`
    let keyID = ''
    let rawKey = ''

    try {
      await page.goto('/settings')
      await page.getByRole('button', { name: 'API Keys' }).click()
      await page.getByRole('button', { name: 'Create API Key' }).click()
      await page.locator('#key-name').fill(name)
      const created = page.waitForResponse((response) =>
        response.url().endsWith('/api/v1/api-keys') && response.request().method() === 'POST',
      )
      await page.getByRole('button', { name: 'Create', exact: true }).click()
      const response = await created
      expect(response.status()).toBe(201)
      const createdKey = await response.json() as JSONRecord
      keyID = String(createdKey.id)
      rawKey = String(createdKey.key)
      await page.getByRole('button', { name: 'Done' }).click()

      expect(rawKey).toMatch(/^vxs_live_/)
      const initialized = await mcpRequest(page, rawKey, 1, 'initialize', {
        protocolVersion: '2025-06-18',
        capabilities: {},
        clientInfo: { name: 'oss-acceptance', version: '1' },
      })
      expect((initialized.result as JSONRecord).protocolVersion).toBeTruthy()

      const toolList = await mcpRequest(page, rawKey, 2, 'tools/list', {})
      const listedTools = (toolList.result as JSONRecord).tools
      const tools: JSONRecord[] = Array.isArray(listedTools)
        ? listedTools.filter((tool): tool is JSONRecord => typeof tool === 'object' && tool !== null && !Array.isArray(tool))
        : []
      expect(tools.some((tool) => tool.name === 'list_media')).toBeTruthy()
      expect(tools.some((tool) => tool.name === 'get_media')).toBeTruthy()

      const call = await mcpRequest(page, rawKey, 3, 'tools/call', {
        name: 'get_media',
        arguments: { media_id: mediaID },
      })
      const callResult = call.result as JSONRecord
      expect(callResult.isError).not.toBe(true)
      const content = Array.isArray(callResult.content) ? callResult.content : []
      expect(content.map((item) => String((item as JSONRecord).text ?? '')).join('\n')).toContain(mediaID)

      const revoked = await page.request.delete(`/api/v1/api-keys/${keyID}`, { headers: headers() })
      expect(revoked.status()).toBe(204)
      keyID = ''
      const rejected = await page.request.post('/mcp', {
        headers: { Authorization: `Bearer ${rawKey}`, 'Content-Type': 'application/json' },
        data: { jsonrpc: '2.0', id: 4, method: 'initialize', params: {} },
      })
      expect(rejected.status()).toBe(401)

      expect((await page.request.get('/api/v1/usage/stats', { headers: headers() })).status()).toBe(200)
      await page.goto('/admin')
      await expect(page.getByRole('heading', { name: 'Administration' })).toBeVisible()
      expect((await page.request.get('/api/v1/admin/system/stats', { headers: headers() })).status()).toBe(200)
    } finally {
      if (keyID) await page.request.delete(`/api/v1/api-keys/${keyID}`, { headers: headers() })
    }
  })
})
