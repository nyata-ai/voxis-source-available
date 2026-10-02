/**
 * Every byte the guide screenshot harness shows a reader.
 *
 * All of it is invented: a fictional small architecture practice reviewing a
 * community library extension. The guide is PUBLIC content, so nothing here may
 * be traceable to a customer, a real recording or a real person — no e-mail
 * address, no hostname, no real session id.
 *
 * Typed against the app's own types so a shape change breaks this file rather
 * than the capture.
 */
import type { AudioAnalysis, MediaItem, MediaStreamUrlResponse } from '../../src/types/media'
import type {
  SpeakerSuggestion,
  TranscriptionDetail,
  Utterance,
} from '../../src/types/transcription'
import type { SummaryCitation, SummaryDetail } from '../../src/types/summary'
import type { UserPreferences } from '../../src/types/settings'

export const TRANSCRIPTION_ID = 'de-mo-se-ss-io-n0-00-01'
export const MEDIA_ID = 'de-mo-me-di-a0-00-00-01'
export const GENERAL_SUMMARY_ID = 'de-mo-su-mm-ar-y0-00-01'
export const KEY_POINTS_SUMMARY_ID = 'de-mo-su-mm-ar-y0-00-02'
const ORG_ID = 'de-mo-or-g0-00-00-00-01'

export const SESSION_TITLE = 'Library extension — design review'
export const SESSION_FILENAME = 'library-extension-review.m4a'

/** 4:02 of audio, which is what the generated WAV is built to. */
export const DURATION_SECONDS = 242

const CREATED_AT = '2026-04-14T09:12:00Z'
const COMPLETED_AT = '2026-04-14T09:16:40Z'
const SUMMARY_CREATED_AT = '2026-04-14T09:18:05Z'

/** Turn text with the speaker index that says it. Times are laid on below, so
 *  the transcript and the generated WAV cannot drift apart. */
const TURNS: ReadonlyArray<readonly [number, string]> = [
  [0, 'Right, let us start. This is the design review for the library extension, and the one thing I want settled today is the reading room.'],
  [1, 'I have the two options drawn up. The first keeps the extension inside the existing footprint. The second pushes six metres into the side garden.'],
  [0, 'Six metres is a lot of garden.'],
  [2, 'It is most of the mature planting, yes. The two limes on that edge are the oldest things on the site. If we go out six metres they come down.'],
  [1, 'We could shift the line to four metres and keep both trees. The reading room loses about fifteen seats.'],
  [0, 'Fifteen out of how many?'],
  [1, 'Ninety in the six metre version. Seventy five in the four metre one.'],
  [3, 'On cost, the four metre option comes in around eleven per cent under the brief. The six metre one is level with it, with nothing left for the furniture package.'],
  [0, 'So the wider room spends the whole contingency before we have bought a single chair.'],
  [3, 'That is the shape of it. Daniel ran the same numbers twice because I did not believe them the first time.'],
  [2, 'The four metre line also lets us keep the path along the hedge, which the community group asked for twice.'],
  [4, 'Can I raise the light? The east elevation in the six metre version puts the reading room in shade from about two in the afternoon.'],
  [1, 'That is right. The four metre version keeps the clerestory working until four.'],
  [0, 'Then I think the four metre option is the one we take forward. Does anyone want to argue for the wider room?'],
  [3, 'Not on cost.'],
  [2, 'Not on the garden.'],
  [4, 'I would want the daylight modelling redone properly before we commit, but I am not arguing against it.'],
  [0, 'Fair. Tomas, can you get the four metre plans updated this week, and can we have the daylight study back before the trustees meet?'],
  [1, 'The plans yes. The study depends on how quickly the consultant turns it round.'],
  [0, 'Chase them tomorrow. Mira, would you put a short note together on the two limes for the trustees pack? Good. Let us stop there.'],
]

function buildUtterances(): Utterance[] {
  const gap = DURATION_SECONDS / TURNS.length
  return TURNS.map(([speaker, text], index) => {
    const start = Number((index * gap).toFixed(2))
    // A short breath between turns, so a `currentTime` landing anywhere inside
    // a turn highlights exactly one row.
    const end = Number((start + gap - 0.4).toFixed(2))
    return { id: index, speaker, language: 'en', start, end, text, words: [] }
  })
}

export const UTTERANCES: Utterance[] = buildUtterances()

/** The turn the capture highlights: far enough in that the reader reads as
 *  mid-session, near enough to the top that the column barely scrolls. */
export const ACTIVE_TURN_INDEX = 3

/** Three names typed in by hand, one row left for the AI suggestion, one row
 *  never named — which is what puts the "Show all 5 speakers" row on the panel. */
export const SPEAKER_MAP: Record<string, string> = {
  '0': 'Ana Vidal',
  '1': 'Tomas Bergh',
  '2': 'Mira Okonjo',
}

export const SUGGESTED_SPEAKER_MAP: Record<string, SpeakerSuggestion> = {
  '3': {
    name: 'Daniel Fisk',
    evidence: 'Daniel ran the same numbers twice because I did not believe them the first time.',
    confidence: 'high',
  },
}

const AUDIO_ANALYSIS: AudioAnalysis = {
  recording_date: '2026-04-14',
  recording_source: 'Handheld recorder',
  encoder: 'Demo encoder',
  trust_level: 'high',
  format_name: 'mov,mp4,m4a',
  codec_name: 'aac',
  sample_rate: 48000,
  channels: 1,
  bitrate: 128000,
  disclaimer:
    'These checks describe the file as uploaded. They are not a legal opinion on the authenticity of the recording.',
  findings: [
    {
      check: 'format_match',
      severity: 'info',
      status: 'pass',
      summary: 'Container and codec agree with the declared type.',
    },
    {
      check: 'codec_validity',
      severity: 'info',
      status: 'pass',
      summary: 'Codec is a known audio codec.',
    },
    {
      check: 'sample_rate',
      severity: 'low',
      status: 'pass',
      summary: 'Sample rate is within the expected range.',
    },
    {
      check: 'bit_depth',
      severity: 'low',
      status: 'skip',
      summary: 'Bit depth is not reported for this codec.',
    },
    {
      check: 'duration_consistency',
      severity: 'medium',
      status: 'pass',
      summary: 'Stream duration matches the container header.',
    },
  ],
}

export const MEDIA: MediaItem = {
  id: MEDIA_ID,
  title: SESSION_TITLE,
  description: '',
  filename: SESSION_FILENAME,
  content_type: 'audio/mp4',
  size: 3_874_112,
  duration: DURATION_SECONDS,
  status: 'completed',
  scan_status: 'scan_clean',
  file_hash: '9f2c1b7a4e6d05938c1a4f7b2e8d3c50a6b9f1d47e2c8a03b5d6e91f2a4c7b80',
  audio_analysis: AUDIO_ANALYSIS,
  encryption_algo: 'AES-256-GCM',
  latest_transcription_id: TRANSCRIPTION_ID,
  audio_available: true,
  created_at: CREATED_AT,
}

const WORD_COUNT = TURNS.reduce((total, [, text]) => total + text.split(/\s+/).length, 0)

export const TRANSCRIPTION: TranscriptionDetail = {
  id: TRANSCRIPTION_ID,
  organization_id: ORG_ID,
  media_id: MEDIA_ID,
  media_filename: SESSION_FILENAME,
  media_title: SESSION_TITLE,
  media_description: '',
  media_status: 'completed',
  status: 'completed',
  languages: ['en'],
  diarization: true,
  speaker_count: 5,
  word_count: WORD_COUNT,
  duration_seconds: DURATION_SECONDS,
  enhance_audio: false,
  created_at: CREATED_AT,
  completed_at: COMPLETED_AT,
  utterances: UTTERANCES,
  full_transcript: TURNS.map(([, text]) => text).join('\n\n'),
  speaker_map: SPEAKER_MAP,
  suggested_speaker_map: SUGGESTED_SPEAKER_MAP,
  // True, so the panel does not fire its one-shot "find names" request and put
  // a spinner in the capture.
  suggestions_generated: true,
}

export const STREAM_URL_RESPONSE: MediaStreamUrlResponse = {
  url: `/api/v1/media/${MEDIA_ID}/stream`,
  expires_at: '2100-01-01T00:00:00Z',
}

const CITATIONS: SummaryCitation[] = [
  { id: 'c1', speaker: 'Tomas Bergh', start_seconds: 12.1, end_seconds: 24.2 },
  { id: 'c2', speaker: 'Daniel Fisk', start_seconds: 84.7, end_seconds: 96.8 },
  { id: 'c3', speaker: 'Ana Vidal', start_seconds: 157.3, end_seconds: 169.4 },
]

const GENERAL_PARAGRAPHS = [
  'The practice reviewed two versions of the library extension. One keeps the new reading room inside the existing footprint of the building; the other pushes six metres into the side garden and seats ninety readers.',
  'The wider version was set aside. It would take down the two mature lime trees on the garden edge, absorb the whole cost contingency before any furniture is bought, and leave the reading room in shade from the middle of the afternoon.',
  'The four metre version keeps both trees, keeps the path along the hedge that the community group asked for, and holds its daylight until about four. The meeting agreed to take it forward, with the daylight modelling to be redone before the design is committed.',
]

const GENERAL_CONTENT = GENERAL_PARAGRAPHS.join('\n\n')

export const GENERAL_SUMMARY: SummaryDetail = {
  id: GENERAL_SUMMARY_ID,
  organization_id: ORG_ID,
  transcription_id: TRANSCRIPTION_ID,
  summary_type: 'general',
  status: 'completed',
  word_count: GENERAL_CONTENT.split(/\s+/).length,
  prompt_tokens: 3140,
  completion_tokens: 268,
  high_stakes: false,
  summary_profile: 'general_professional',
  review_status: 'passed',
  created_at: SUMMARY_CREATED_AT,
  completed_at: SUMMARY_CREATED_AT,
  content: GENERAL_CONTENT,
  citations: CITATIONS,
  structured_content: {
    schema_version: 'structured_summary_v1',
    matrix_language: 'en',
    paragraphs: GENERAL_PARAGRAPHS.map((text, index) => ({
      id: `p${index + 1}`,
      text,
      speaker: null,
      citation_ids: [CITATIONS[index % CITATIONS.length].id],
    })),
  },
}

const KEY_POINTS = [
  'Two versions of the reading room were compared: four metres into the garden with seventy five seats, and six metres with ninety.',
  'The six metre line would take down the two mature lime trees on the garden edge.',
  'The four metre option comes in about eleven per cent under the brief; the six metre option leaves nothing for the furniture package.',
  'The six metre reading room falls into shade from about two in the afternoon; the four metre version keeps the clerestory working until four.',
  'The four metre option was agreed as the one to take forward, subject to the daylight modelling being redone.',
]

const KEY_POINTS_CONTENT = KEY_POINTS.map((point) => `- ${point}`).join('\n')

export const KEY_POINTS_SUMMARY: SummaryDetail = {
  id: KEY_POINTS_SUMMARY_ID,
  organization_id: ORG_ID,
  transcription_id: TRANSCRIPTION_ID,
  summary_type: 'key_points',
  status: 'completed',
  word_count: KEY_POINTS_CONTENT.split(/\s+/).length,
  prompt_tokens: 3140,
  completion_tokens: 191,
  high_stakes: false,
  summary_profile: 'general_professional',
  review_status: 'passed',
  created_at: SUMMARY_CREATED_AT,
  completed_at: SUMMARY_CREATED_AT,
  content: KEY_POINTS_CONTENT,
  citations: CITATIONS,
  structured_content: {
    schema_version: 'structured_summary_v1',
    matrix_language: 'en',
    items: KEY_POINTS.map((text, index) => ({
      id: `k${index + 1}`,
      text,
      speaker: null,
      citation_ids: [CITATIONS[index % CITATIONS.length].id],
    })),
  },
}

/** Only the two completed briefings exist. Action Items and Questions &
 *  Answers have never been generated, which is what gives the tab row its
 *  mixed status dots. */
export const SUMMARIES: SummaryDetail[] = [GENERAL_SUMMARY, KEY_POINTS_SUMMARY]

export const SUMMARY_BY_ID: Record<string, SummaryDetail> = {
  [GENERAL_SUMMARY_ID]: GENERAL_SUMMARY,
  [KEY_POINTS_SUMMARY_ID]: KEY_POINTS_SUMMARY,
}

export const PREFERENCES: UserPreferences = {
  theme: 'light',
  default_languages: ['en'],
  default_diarization: true,
  default_summary_type: 'general',
  default_export_format: 'pdf',
  playback_speed: 1,
  high_stakes_summaries: false,
  auto_briefings: false,
  summary_profile: 'general_professional',
  ui_language: 'en',
}

/** Profiles on, so the briefing band renders its Profile select. Billing off,
 *  so no credit chrome joins the shell. BAP off: the Berita Acara row needs a
 *  completed Questions & Answers briefing, which this session deliberately
 *  does not have. */
export const FEATURES = {
  enhance_audio: true,
  deep_filter: true,
  summary_high_stakes: false,
  summary_structured_output: true,
  summary_profiles: true,
  summary_shared_analysis: false,
  account_security: false,
  billing: false,
  custom_vocabulary: false,
  bap_export: false,
}

/** `email` is what the header prints under the display name, so it carries a
 *  workspace label rather than an address. */
export const CURRENT_USER = {
  id: '00000000-0000-4000-8000-000000000001',
  name: 'Demo Reviewer',
  email: 'Demo workspace',
  organization: { id: ORG_ID, name: 'Demo workspace', tier: 'pro' },
}

// A controlled public-guide example. It never reaches the product API or a
// provider, and contains only invented information.
export const ACTIVITY = {
  items: [
    {
      kind: 'transcription',
      ref_id: 'guide-activity-example',
      title: 'Sample planning meeting',
      stage: 'transcribing',
      status: 'failed',
      error_message: 'Transcription could not complete. Check the provider setting and retry.',
      started_at: '2026-09-20T09:00:00Z',
    },
  ],
}

export const ADMIN_ME = { is_admin: false }
