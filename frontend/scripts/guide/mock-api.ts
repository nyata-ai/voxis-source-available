/** Synthetic API for the guide capture harness. It never reaches an installation. */
import type { Page, Route } from '@playwright/test'
import {
  ACTIVITY,
  ADMIN_ME,
  CURRENT_USER,
  DURATION_SECONDS,
  FEATURES,
  MEDIA,
  MEDIA_ID,
  PREFERENCES,
  SESSION_FILENAME,
  SESSION_TITLE,
  STREAM_URL_RESPONSE,
  SUMMARIES,
  SUMMARY_BY_ID,
  TRANSCRIPTION,
  TRANSCRIPTION_ID,
} from './fixtures'

const SAMPLE_RATE = 8000
const audioData = silentWav(DURATION_SECONDS)

function silentWav(seconds: number): Buffer {
  const dataLength = SAMPLE_RATE * Math.round(seconds)
  const buffer = Buffer.alloc(44 + dataLength)
  buffer.write('RIFF', 0, 'ascii')
  buffer.writeUInt32LE(36 + dataLength, 4)
  buffer.write('WAVE', 8, 'ascii')
  buffer.write('fmt ', 12, 'ascii')
  buffer.writeUInt32LE(16, 16)
  buffer.writeUInt16LE(1, 20)
  buffer.writeUInt16LE(1, 22)
  buffer.writeUInt32LE(SAMPLE_RATE, 24)
  buffer.writeUInt32LE(SAMPLE_RATE, 28)
  buffer.writeUInt16LE(1, 32)
  buffer.writeUInt16LE(8, 34)
  buffer.write('data', 36, 'ascii')
  buffer.writeUInt32LE(dataLength, 40)
  buffer.fill(0x80, 44)
  return buffer
}

function json(route: Route, body: unknown, status = 200): Promise<void> {
  return route.fulfill({ status, contentType: 'application/json', headers: { 'cache-control': 'no-store' }, body: JSON.stringify(body) })
}

function audio(route: Route): Promise<void> {
  return route.fulfill({ status: 200, headers: { 'content-type': 'audio/wav', 'content-length': String(audioData.length), 'accept-ranges': 'bytes' }, body: audioData })
}

const staticGets: Record<string, unknown> = {
  '/features': FEATURES,
  '/me': CURRENT_USER,
  '/admin/me': ADMIN_ME,
  '/settings/preferences': PREFERENCES,
  '/activity': ACTIVITY,
  '/dashboard/stats': { total_media: 1, processing_count: 0, encrypting_count: 0, transcribing_count: 0, completed_transcriptions: 1, completed_this_month: 1, completed_last_month: 0, seconds_this_month: DURATION_SECONDS, last_completed_at: '2026-04-14T09:16:40Z' },
  '/usage/stats': { total_duration_seconds: DURATION_SECONDS, total_prompt_tokens: 3140, total_completion_tokens: 268, total_thinking_tokens: 0, user_storage: { used_bytes: 3874112, limit_bytes: 1073741824, state: 'available' } },
  '/recordings/interrupted': { items: [] },
}

export async function mockApi(page: Page): Promise<string[]> {
  const unmocked: string[] = []
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api\/v1/, '')
    if (request.method() === 'GET') {
      const staticBody = staticGets[path]
      if (staticBody !== undefined) return json(route, staticBody)
      if (path === '/recordings/active') return json(route, { error: 'not_found' }, 404)
      if (path === '/media') return json(route, { items: [MEDIA], total: 1 })
      if (path === '/transcriptions') return json(route, { items: [TRANSCRIPTION], total: 1 })
      if (path === `/transcriptions/${TRANSCRIPTION_ID}`) return json(route, TRANSCRIPTION)
      if (path === `/transcriptions/${TRANSCRIPTION_ID}/summaries`) return json(route, SUMMARIES)
      if (path === `/media/${MEDIA_ID}`) return json(route, MEDIA)
      if (path === `/media/${MEDIA_ID}/stream-url`) return json(route, STREAM_URL_RESPONSE)
      if (path === `/media/${MEDIA_ID}/stream`) return audio(route)
      if (path === '/search') {
        const cursor = url.searchParams.get('transcript_cursor')
        return json(route, {
          items: [],
          total: 0,
          query: url.searchParams.get('search') ?? '',
          search_mode: 'lexical',
          semantic_available: false,
          transcript_items: [
            {
              id: MEDIA_ID,
              entity_type: 'media',
              title: SESSION_TITLE,
              filename: SESSION_FILENAME,
              status: 'completed',
              created_at: '2026-04-14T09:16:40Z',
              latest_transcription_id: cursor ? `${TRANSCRIPTION_ID}-earlier` : TRANSCRIPTION_ID,
              score: 1,
              match_fields: ['transcript'],
              snippet: cursor
                ? 'The earlier completed transcript also discusses the library extension.'
                : 'The design review discusses the library extension and its reading room.',
            },
          ],
          ...(cursor
            ? { transcript_complete: true }
            : { next_transcript_cursor: 'synthetic-next-page', transcript_complete: false }),
        })
      }
      const summaryMatch = /^\/summaries\/([^/]+)$/.exec(path)
      const summary = summaryMatch ? SUMMARY_BY_ID[summaryMatch[1]] : undefined
      if (summary) return json(route, summary)
    }
    unmocked.push(`${request.method()} ${path}`)
    return json(route, { error: 'not_found' }, 404)
  })
  return unmocked
}
