import { describe, expect, it } from 'vitest'
import { getStructuredSummaryContent, parseQuestionAnswerContent } from './summary-content'
import type { SummaryDetail } from '@/types/summary'

const baseSummary: SummaryDetail = {
  id: 'summary-1',
  organization_id: 'org-1',
  transcription_id: 'transcription-1',
  summary_type: 'general',
  status: 'completed',
  word_count: 1,
  prompt_tokens: 1,
  completion_tokens: 1,
  high_stakes: false,
  review_status: 'skipped',
  created_at: '2026-08-24T00:00:00Z',
}

describe('parseQuestionAnswerContent', () => {
  it('parses multiple Q&A pairs without blank lines between them', () => {
    expect(
      parseQuestionAnswerContent(
        'Q: Can we ship today?\nA: Yes, after QA signs off.\nQ: Who owns follow-up?\nA: Maya owns it.'
      )
    ).toEqual([
      { question: 'Can we ship today?', answer: 'Yes, after QA signs off.' },
      { question: 'Who owns follow-up?', answer: 'Maya owns it.' },
    ])
  })

  it('keeps multiline questions and answers', () => {
    expect(
      parseQuestionAnswerContent(
        'Q: Can we ship today?\nwith the release branch?\nA: Yes.\nUse the signed tag.'
      )
    ).toEqual([
      {
        question: 'Can we ship today?\nwith the release branch?',
        answer: 'Yes.\nUse the signed tag.',
      },
    ])
  })

  it('returns null for malformed content', () => {
    expect(parseQuestionAnswerContent('Q: Missing answer')).toBeNull()
    expect(parseQuestionAnswerContent('A: Missing question')).toBeNull()
  })
})

describe('getStructuredSummaryContent', () => {
  it('accepts the typed content matching the summary type', () => {
    const content = getStructuredSummaryContent({
      ...baseSummary,
      summary_type: 'key_points',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        items: [{ id: 'k1', text: 'Approved.', speaker: null, citation_ids: ['u1'] }],
      },
    })

    expect(content).toMatchObject({
      matrix_language: 'en',
      items: [{ id: 'k1', citation_ids: ['u1'] }],
    })
  })

  it('accepts a JSON-encoded structured content root for additive API compatibility', () => {
    const content = getStructuredSummaryContent({
      ...baseSummary,
      structured_content: JSON.stringify({
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [{ id: 'g1', text: 'Approved.', speaker: null, citation_ids: ['u1'] }],
      }) as unknown as SummaryDetail['structured_content'],
    })

    expect(content).toMatchObject({ paragraphs: [{ id: 'g1', text: 'Approved.' }] })
  })

  it('rejects malformed or mismatched structured content', () => {
    const malformed = getStructuredSummaryContent({
      ...baseSummary,
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [{ id: 'g1', text: 'No evidence.', speaker: null, citation_ids: [] }],
      },
    })
    const mismatched = getStructuredSummaryContent({
      ...baseSummary,
      summary_type: 'q_and_a',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [{ id: 'g1', text: 'Not Q&A.', speaker: null, citation_ids: ['u1'] }],
      },
    })

    expect(malformed).toBeNull()
    expect(mismatched).toBeNull()
  })
})
