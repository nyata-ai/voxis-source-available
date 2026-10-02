import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { SummaryViewer } from './SummaryViewer'
import type { SummaryDetail } from '@/types/summary'

const baseSummary: SummaryDetail = {
  id: 'sum-1',
  organization_id: 'org-1',
  transcription_id: 'tx-1',
  summary_type: 'general',
  status: 'completed',
  word_count: 120,
  prompt_tokens: 500,
  completion_tokens: 200,
  high_stakes: false,
  review_status: 'skipped',
  created_at: '2026-02-10T10:00:00Z',
  completed_at: '2026-02-10T10:02:00Z',
  content: 'First paragraph of the summary.\n\nSecond paragraph with more details.',
}

const writeTextMock = vi.fn().mockResolvedValue(undefined)

describe('SummaryViewer', () => {
  beforeEach(() => {
    writeTextMock.mockClear()
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: writeTextMock },
      writable: true,
      configurable: true,
    })
  })

  it('renders general summary as paragraphs', () => {
    render(<SummaryViewer summary={baseSummary} />)
    // General summaries split on double newlines into paragraphs
    expect(screen.getByText('First paragraph of the summary.')).toBeInTheDocument()
    expect(screen.getByText('Second paragraph with more details.')).toBeInTheDocument()
    // Each should be rendered as a <p> element
    const p1 = screen.getByText('First paragraph of the summary.')
    const p2 = screen.getByText('Second paragraph with more details.')
    expect(p1.tagName).toBe('P')
    expect(p2.tagName).toBe('P')
  })

  it('renders key_points as bulleted list', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'key_points',
      content: '- First key point\n- Second key point\n* Third key point',
    }
    render(<SummaryViewer summary={summary} />)
    expect(screen.getByText('First key point')).toBeInTheDocument()
    expect(screen.getByText('Second key point')).toBeInTheDocument()
    expect(screen.getByText('Third key point')).toBeInTheDocument()
    // ARIA list semantics preserved for screen readers
    expect(screen.getByRole('list')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('renders key point evidence separately', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'key_points',
      content: '- Budget was approved. Evidence: "Alice said approved"',
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('Budget was approved.')).toBeInTheDocument()
    expect(screen.getByText('Evidence: Alice said approved')).toBeInTheDocument()
  })

  it('splits legacy key points on the final canonical Evidence delimiter', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'key_points',
      content:
        '- Evidence: ownership uses the word Evidence: in the point. Evidence: "Sari approved it."',
    }
    render(<SummaryViewer summary={summary} />)

    expect(
      screen.getByText('Evidence: ownership uses the word Evidence: in the point.')
    ).toBeInTheDocument()
    expect(screen.getByText('Evidence: Sari approved it.')).toBeInTheDocument()
  })

  it('renders action_items as numbered blocks', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'action_items',
      content: '1. Complete the report\n2. Send to client\n3. Follow up next week',
    }
    render(<SummaryViewer summary={summary} />)
    expect(screen.getByText('Complete the report')).toBeInTheDocument()
    expect(screen.getByText('Send to client')).toBeInTheDocument()
    expect(screen.getByText('Follow up next week')).toBeInTheDocument()
    // Each item has a number prefix
    expect(screen.getByText('1.')).toBeInTheDocument()
    expect(screen.getByText('2.')).toBeInTheDocument()
    expect(screen.getByText('3.')).toBeInTheDocument()
    // ARIA list semantics preserved for screen readers
    expect(screen.getByRole('list')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('renders structured action item fields', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'action_items',
      content:
        '1. Type: Decision; Owner: Sari; Due: Not stated in the transcript.; Item: Approve the filing schedule; Evidence: "Sari approved the schedule".',
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('Decision')).toBeInTheDocument()
    expect(screen.getByText('Approve the filing schedule')).toBeInTheDocument()
    expect(screen.getByText('Owner: Sari')).toBeInTheDocument()
    expect(screen.getByText('Due: Not stated in the transcript.')).toBeInTheDocument()
    expect(screen.getByText('Evidence: Sari approved the schedule')).toBeInTheDocument()
  })

  it('prefers structured general content and seeks from a timestamped citation', async () => {
    const onCitationSeek = vi.fn()
    const summary: SummaryDetail = {
      ...baseSummary,
      content: 'Canonical general summary.\n\nEvidence: "Sari approved the filing schedule."',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [
          {
            id: 'g1',
            text: 'Sari approved the filing schedule.',
            speaker: 'Sari',
            citation_ids: ['u000001'],
          },
        ],
      },
      citations: [{ id: 'u000001', speaker: 'Sari', start_seconds: 12.4, end_seconds: 18.9 }],
    }
    render(<SummaryViewer summary={summary} onCitationSeek={onCitationSeek} />)

    expect(screen.getByText('Sari approved the filing schedule.')).toBeInTheDocument()
    expect(screen.queryByText('Canonical general summary.')).not.toBeInTheDocument()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Show sources' }))
    })

    const citation = screen.getByRole('button', { name: 'Citation u000001, Sari, 00:12' })
    await act(async () => {
      fireEvent.click(citation)
    })

    expect(onCitationSeek).toHaveBeenCalledWith(12.4)
  })

  it('uses canonical content for copy when structured content is displayed', async () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      content: 'Canonical content for export and copy.',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [
          { id: 'g1', text: 'Structured display content.', speaker: null, citation_ids: ['u1'] },
        ],
      },
    }
    render(<SummaryViewer summary={summary} />)

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /copy/i }))
    })

    expect(writeTextMock).toHaveBeenCalledWith('Canonical content for export and copy.')
  })

  it('hides citation chips until Show sources is pressed, then flips the toggle label', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [
          { id: 'g1', text: 'Structured display content.', speaker: null, citation_ids: ['u9'] },
        ],
      },
      citations: [{ id: 'u9', speaker: 'Sari' }],
    }
    render(<SummaryViewer summary={summary} />)

    // Clean reading view by default — the body renders, the provenance does not.
    expect(screen.getByText('Structured display content.')).toBeInTheDocument()
    expect(screen.queryByText('Citation u9, Sari')).not.toBeInTheDocument()

    const toggle = screen.getByRole('button', { name: 'Show sources' })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')

    fireEvent.click(toggle)

    expect(screen.getByText('Citation u9, Sari')).toBeInTheDocument()
    const pressed = screen.getByRole('button', { name: 'Hide sources' })
    expect(pressed).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(pressed)

    expect(screen.queryByText('Citation u9, Sari')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show sources' })).toBeInTheDocument()
  })

  it('omits the Show sources toggle for a legacy summary with no structured citations', () => {
    render(<SummaryViewer summary={baseSummary} />)

    expect(screen.getByText('First paragraph of the summary.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /sources/i })).not.toBeInTheDocument()
  })

  it('omits the Show sources toggle while loading', () => {
    render(<SummaryViewer isLoading />)

    expect(screen.queryByRole('button', { name: /sources/i })).not.toBeInTheDocument()
  })

  it('renders structured key points with read-only citations when seeking is unavailable', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'key_points',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        items: [
          { id: 'k1', text: 'The budget was approved.', speaker: 'Chen', citation_ids: ['u2'] },
        ],
      },
      citations: [{ id: 'u2', speaker: 'Chen', start_seconds: 30 }],
    }
    render(<SummaryViewer summary={summary} />)
    fireEvent.click(screen.getByRole('button', { name: 'Show sources' }))

    expect(screen.getByText('The budget was approved.')).toBeInTheDocument()
    expect(screen.getByText('Citation u2, Chen, 00:30')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Citation u2, Chen, 00:30' })
    ).not.toBeInTheDocument()
  })

  it.each([-1, Number.NaN])(
    'does not seek or display an invalid citation timestamp: %s',
    (start) => {
      const onCitationSeek = vi.fn()
      const summary: SummaryDetail = {
        ...baseSummary,
        structured_content: {
          schema_version: 'structured_summary_v1',
          matrix_language: 'en',
          paragraphs: [
            { id: 'g1', text: 'Structured content.', speaker: null, citation_ids: ['u5'] },
          ],
        },
        citations: [{ id: 'u5', speaker: 'Sari', start_seconds: start }],
      }
      render(<SummaryViewer summary={summary} onCitationSeek={onCitationSeek} />)
      fireEvent.click(screen.getByRole('button', { name: 'Show sources' }))

      expect(screen.getByText('Citation u5, Sari')).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Citation u5, Sari' })).not.toBeInTheDocument()
      expect(onCitationSeek).not.toHaveBeenCalled()
    }
  )

  it('renders combined structured action item kinds and fields', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'action_items',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        items: [
          {
            id: 'a1',
            kind: 'open_issue',
            text: 'Confirm the filing deadline.',
            owner: 'Maya',
            deadline: null,
            speaker: null,
            citation_ids: ['u3'],
          },
        ],
      },
      citations: [{ id: 'u3' }],
    }
    render(<SummaryViewer summary={summary} />)
    fireEvent.click(screen.getByRole('button', { name: 'Show sources' }))

    expect(screen.getByText('Open issue')).toBeInTheDocument()
    expect(screen.getByText('Confirm the filing deadline.')).toBeInTheDocument()
    expect(screen.getByText('Owner: Maya')).toBeInTheDocument()
    expect(screen.getByText('Due: Not stated in the transcript.')).toBeInTheDocument()
    expect(screen.getByText('Citation u3')).toBeInTheDocument()
  })

  it('shows localized missing owner and due values for structured action items', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'action_items',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        items: [
          {
            id: 'a2',
            kind: 'next_step',
            text: 'Confirm the filing deadline.',
            owner: null,
            deadline: null,
            speaker: null,
            citation_ids: ['u3'],
          },
        ],
      },
      citations: [{ id: 'u3' }],
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('Next step')).toBeInTheDocument()
    expect(screen.getByText('Owner: Not stated in the transcript.')).toBeInTheDocument()
    expect(screen.getByText('Due: Not stated in the transcript.')).toBeInTheDocument()
  })

  it('shows a degradation notice only for a non-empty string code array', () => {
    const { rerender } = render(
      <SummaryViewer
        summary={{
          ...baseSummary,
          generation_metadata: { degradation_codes: ['source_segments_truncated'] },
        }}
      />
    )

    expect(screen.getByRole('status')).toHaveTextContent(
      'This summary was completed with fallback or normalized output. Review it against the transcript.'
    )

    rerender(
      <SummaryViewer summary={{ ...baseSummary, generation_metadata: { degradation_codes: [] } }} />
    )

    expect(screen.queryByRole('status')).not.toBeInTheDocument()

    rerender(
      <SummaryViewer
        summary={{
          ...baseSummary,
          generation_metadata: {
            degradation_codes: ['source_segments_truncated', 1] as unknown as string[],
          },
        }}
      />
    )

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('renders structured Q&A in the accepted speaker voice', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'q_and_a',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        items: [
          {
            id: 'q1',
            question: 'What changed?',
            question_speaker: 'Budi',
            answer: 'The launch moved to Friday.',
            answer_speaker: 'Sari',
            answer_status: 'answered',
            answer_exact_quote: null,
            citation_ids: ['u4'],
          },
        ],
      },
      citations: [{ id: 'u4' }],
    }
    render(<SummaryViewer summary={summary} />)
    fireEvent.click(screen.getByRole('button', { name: 'Show sources' }))

    expect(screen.getByText('[Budi] What changed?')).toBeInTheDocument()
    expect(screen.getByText('[Sari] The launch moved to Friday.')).toBeInTheDocument()
    expect(screen.getByText('Citation u4')).toBeInTheDocument()
  })

  it('falls back to canonical prose when structured content is malformed', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      content: 'Canonical fallback paragraph.\n\nEvidence: "Approved."',
      structured_content: {
        schema_version: 'structured_summary_v1',
        matrix_language: 'en',
        paragraphs: [{ id: 'g1', text: 'Missing citations.', speaker: null, citation_ids: [] }],
      },
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('Canonical fallback paragraph.')).toBeInTheDocument()
    expect(screen.getByText('Evidence: "Approved."')).toBeInTheDocument()
  })

  it.each([
    {
      type: 'general' as const,
      content: 'The filing was approved.\n\nEvidence: "Sari approved it."',
      expected: ['The filing was approved.', 'Evidence: "Sari approved it."'],
    },
    {
      type: 'key_points' as const,
      content: '- The filing was approved. Evidence: "Sari approved it."',
      expected: ['The filing was approved.', 'Evidence: Sari approved it.'],
    },
    {
      type: 'action_items' as const,
      content:
        '1. Type: Decision; Owner: Sari; Due: Friday; Item: Approve filing; Evidence: "Sari approved it."',
      expected: [
        'Decision',
        'Owner: Sari',
        'Due: Friday',
        'Approve filing',
        'Evidence: Sari approved it.',
      ],
    },
    {
      type: 'q_and_a' as const,
      content: 'Q: What changed?\nA: The filing was approved.',
      expected: ['What changed?', 'The filing was approved.'],
    },
  ])('preserves the legacy $type canonical text contract', ({ type, content, expected }) => {
    render(<SummaryViewer summary={{ ...baseSummary, summary_type: type, content }} />)

    for (const text of expected) {
      const rendered = text.startsWith('Evidence:')
        ? screen.getByText((_, element) => element?.textContent === text)
        : screen.getByText(text)
      expect(rendered).toBeInTheDocument()
    }
  })

  it('renders q_and_a as question and answer blocks', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'q_and_a',
      content:
        'Q: [Budi] What changed?\nA: The launch moved to Friday.\n\nQ: Who owns follow-up?\nA: [Sari] I will send the notes.',
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('Questions & Answers')).toBeInTheDocument()
    expect(screen.getByText('[Budi] What changed?')).toBeInTheDocument()
    expect(screen.getByText('The launch moved to Friday.')).toBeInTheDocument()
    expect(screen.getByText('Who owns follow-up?')).toBeInTheDocument()
    expect(screen.getByText('[Sari] I will send the notes.')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getAllByText('Q')).toHaveLength(2)
    expect(screen.getAllByText('A')).toHaveLength(2)
  })

  it('renders consecutive q_and_a pairs without blank lines as separate blocks', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'q_and_a',
      content: 'Q: First?\nA: First answer.\nQ: Second?\nA: Second answer.',
    }
    render(<SummaryViewer summary={summary} />)

    expect(screen.getByText('First?')).toBeInTheDocument()
    expect(screen.getByText('First answer.')).toBeInTheDocument()
    expect(screen.getByText('Second?')).toBeInTheDocument()
    expect(screen.getByText('Second answer.')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getAllByText('Q')).toHaveLength(2)
    expect(screen.getAllByText('A')).toHaveLength(2)
  })

  it('falls back to paragraphs when q_and_a content is malformed', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      summary_type: 'q_and_a',
      content: 'Q: What changed?\nThe launch moved to Friday.',
    }
    render(<SummaryViewer summary={summary} />)

    const paragraph = screen.getByText(
      (_, element) =>
        element?.tagName === 'P' &&
        element.textContent === 'Q: What changed?\nThe launch moved to Friday.'
    )
    expect(paragraph.tagName).toBe('P')
  })

  it('renders empty state', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      content: undefined,
    }
    render(<SummaryViewer summary={summary} />)
    expect(screen.getByText('No summary content')).toBeInTheDocument()
  })

  it('renders pending state', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      status: 'pending',
      word_count: 0,
      content: undefined,
      completed_at: undefined,
    }
    render(<SummaryViewer summary={summary} />)
    expect(screen.getByText('Processing...')).toBeInTheDocument()
  })

  it('renders failed state', () => {
    const summary: SummaryDetail = {
      ...baseSummary,
      status: 'failed',
      word_count: 0,
      content: undefined,
      error_message: 'Token limit exceeded',
      completed_at: undefined,
    }
    render(<SummaryViewer summary={summary} />)
    expect(screen.getByText('Token limit exceeded')).toBeInTheDocument()
  })

  it('supports copy to clipboard', async () => {
    render(<SummaryViewer summary={baseSummary} />)

    const copyButton = screen.getByRole('button', { name: /copy/i })
    await act(async () => {
      fireEvent.click(copyButton)
    })

    expect(writeTextMock).toHaveBeenCalledWith(baseSummary.content)
    expect(screen.getByText('Copied!')).toBeInTheDocument()
  })

  it('renders a loading line and no content while isLoading', () => {
    render(<SummaryViewer isLoading />)
    expect(screen.getByText('Loading summary...')).toBeInTheDocument()
    expect(screen.queryByText('First paragraph of the summary.')).not.toBeInTheDocument()
  })

  it('prefers the loading line over stale content while isLoading', () => {
    render(<SummaryViewer summary={baseSummary} isLoading />)
    expect(screen.getByText('Loading summary...')).toBeInTheDocument()
    expect(screen.queryByText('First paragraph of the summary.')).not.toBeInTheDocument()
  })

  it('omits the card chrome and the type heading when headless, but keeps Copy', () => {
    const { container } = render(<SummaryViewer summary={baseSummary} headless />)

    expect(screen.queryByText('General Summary')).not.toBeInTheDocument()
    // The Card primitive is the only thing carrying `bg-card` here.
    expect(container.querySelector('.bg-card')).toBeNull()
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
    expect(screen.getByText('First paragraph of the summary.')).toBeInTheDocument()
  })

  it('keeps the type heading when not headless', () => {
    render(<SummaryViewer summary={baseSummary} />)
    expect(screen.getByText('General Summary')).toBeInTheDocument()
  })

  // The reader's briefing panel already caps the band at 78ch; a 65ch prose cap
  // inside it would render the body narrower than the artboard asks for.
  it('lets the general body fill its host when headless, and caps it in the card', () => {
    const { container, unmount } = render(<SummaryViewer summary={baseSummary} headless />)
    expect(container.querySelector('.max-w-prose')).toBeNull()
    unmount()

    const card = render(<SummaryViewer summary={baseSummary} />)
    expect(card.container.querySelector('.max-w-prose')).not.toBeNull()
  })

  it('paints a failed lens coral inside the reader and destructive in the card', () => {
    const failed = { ...baseSummary, status: 'failed' as const, error_message: 'Boom' }
    const { unmount } = render(<SummaryViewer summary={failed} headless />)
    expect(screen.getByText('Boom')).toHaveClass('text-coral')
    unmount()

    render(<SummaryViewer summary={failed} />)
    expect(screen.getByText('Boom')).toHaveClass('text-destructive')
  })

  it('renders the actions slot next to Copy', () => {
    render(
      <SummaryViewer
        summary={baseSummary}
        headless
        actions={<button type="button">Regenerate</button>}
      />
    )

    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
  })

  it('renders the actions slot even when there is no content to copy', () => {
    render(
      <SummaryViewer
        summary={{ ...baseSummary, content: undefined }}
        headless
        actions={<button type="button">Regenerate</button>}
      />
    )

    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
    expect(screen.getByText('No summary content')).toBeInTheDocument()
  })

  it('renders the actions slot on the failed branch — the branch that needs a retry', () => {
    render(
      <SummaryViewer
        summary={{ ...baseSummary, status: 'failed', content: undefined, error_message: 'Boom' }}
        headless
        actions={<button type="button">Regenerate</button>}
      />
    )

    expect(screen.getByText('Boom')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
  })

  it('renders the actions slot on the pending and empty branches', () => {
    const { unmount } = render(
      <SummaryViewer
        summary={{ ...baseSummary, status: 'pending', content: undefined }}
        actions={<button type="button">Regenerate</button>}
      />
    )
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
    unmount()

    render(<SummaryViewer actions={<button type="button">Regenerate</button>} />)
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeInTheDocument()
  })

  it('renders the empty state for a completed summary with no content when not loading', () => {
    render(<SummaryViewer summary={{ ...baseSummary, content: undefined }} isLoading={false} />)
    expect(screen.getByText('No summary content')).toBeInTheDocument()
  })

  it('renders the empty state when no summary is supplied and nothing is loading', () => {
    render(<SummaryViewer />)
    expect(screen.getByText('No summary content')).toBeInTheDocument()
  })

  it('does not crash when clipboard access is denied', async () => {
    writeTextMock.mockRejectedValueOnce(new DOMException('Clipboard access denied'))
    render(<SummaryViewer summary={baseSummary} />)

    const copyButton = screen.getByRole('button', { name: /copy/i })
    await act(async () => {
      fireEvent.click(copyButton)
    })

    expect(writeTextMock).toHaveBeenCalledWith(baseSummary.content)
    // Should not show "Copied!" since the write failed
    expect(screen.queryByText('Copied!')).not.toBeInTheDocument()
    // The copy button should still be present (component did not crash)
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
  })
})
