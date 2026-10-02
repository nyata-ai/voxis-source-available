import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BriefingPane } from './BriefingPane'
import { toLenses } from './lenses'
import type { SessionBriefingControls, SessionLens } from './session-view'
import type { SummaryDetail, SummaryType } from '@/types/summary'
import type { SummaryProfile } from '@/types/settings'

interface SummaryQueryResult {
  data: SummaryDetail | undefined
  isLoading: boolean
  isError?: boolean
}

const useSummaryDetailMock = vi.fn(
  (_id: string): SummaryQueryResult => ({ data: undefined, isLoading: false })
)

vi.mock('@/hooks/useSummary', () => ({
  useSummaryDetail: (id: string) => useSummaryDetailMock(id),
}))

function summary(overrides: Partial<SummaryDetail> & { id: string }): SummaryDetail {
  return {
    organization_id: 'org-1',
    transcription_id: 'tx-1',
    summary_type: 'general',
    status: 'completed',
    word_count: 10,
    prompt_tokens: 100,
    completion_tokens: 50,
    high_stakes: false,
    review_status: 'skipped',
    created_at: '2026-08-01T10:00:00Z',
    ...overrides,
  }
}

function controls(overrides: Partial<SessionBriefingControls> = {}): SessionBriefingControls {
  return {
    generateType: vi.fn(),
    generateTypeError: null,
    generateAll: vi.fn(),
    isGeneratingAll: false,
    error: null,
    regenerate: vi.fn(),
    regenerateError: null,
    ...overrides,
  }
}

interface PaneProps {
  lenses: SessionLens[]
  briefing?: SessionBriefingControls
  activeLens?: SummaryType
  onActiveLensChange?: (type: SummaryType) => void
  defaultSummaryProfile?: SummaryProfile
  summaryProfilesEnabled?: boolean
  onCitationSeek?: (startSeconds: number) => void
}

/** The tab is controlled by the reader shell, so every render supplies it. */
function renderPane(props: PaneProps) {
  const briefing = props.briefing ?? controls()
  const onActiveLensChange = props.onActiveLensChange ?? vi.fn()
  const element = (next: PaneProps) => (
    <BriefingPane
      lenses={next.lenses}
      briefing={briefing}
      activeLens={next.activeLens ?? 'general'}
      onActiveLensChange={onActiveLensChange}
      defaultSummaryProfile={next.defaultSummaryProfile}
      summaryProfilesEnabled={next.summaryProfilesEnabled}
      onCitationSeek={next.onCitationSeek}
    />
  )
  const view = render(element(props))
  /** The shell owns `activeLens`, so switching tabs is a re-render from above. */
  const rerenderPane = (next: Partial<PaneProps> = {}) =>
    view.rerender(element({ ...props, ...next }))
  return { ...view, briefing, onActiveLensChange, rerenderPane }
}

const allGenerated = toLenses([
  summary({ id: 's-general', summary_type: 'general' }),
  summary({ id: 's-key', summary_type: 'key_points' }),
  summary({ id: 's-actions', summary_type: 'action_items' }),
  summary({ id: 's-qa', summary_type: 'q_and_a' }),
])

/** One lens's tab, by its localized full-type label. */
function tab(name: string) {
  return screen.getByRole('tab', { name: new RegExp(name) })
}

describe('BriefingPane', () => {
  beforeEach(() => {
    useSummaryDetailMock.mockReset()
    useSummaryDetailMock.mockReturnValue({ data: undefined, isLoading: false, isError: false })
  })

  it('anchors the band so a ?lens deep link has something to scroll to', () => {
    const { container } = renderPane({ lenses: allGenerated })
    const band = container.querySelector('[data-testid="briefing-band"]')
    expect(band).toHaveAttribute('id', 'briefing')
    expect(band).toHaveAttribute('aria-labelledby', 'briefing-pane-title')
  })

  it('renders one tab per lens in SUMMARY_TYPE_ORDER', () => {
    renderPane({ lenses: allGenerated })

    const tabs = screen.getAllByRole('tab')
    expect(tabs.map((element) => element.textContent)).toEqual([
      'General Summary',
      'Key Points',
      'Action Items',
      'Questions & Answers',
    ])
  })

  describe('tab accessibility', () => {
    // twMerge has to let the forest pill beat the shadcn default, or the
    // selected tab is indistinguishable from the other three.
    it('paints the selected tab forest', () => {
      renderPane({ lenses: allGenerated, activeLens: 'key_points' })
      expect(tab('Key Points').className).toContain('data-[state=active]:bg-forest')
      expect(tab('Key Points').className).not.toContain('data-[state=active]:bg-background')
    })

    it('names the tab list and marks only the active tab selected', () => {
      renderPane({ lenses: allGenerated, activeLens: 'key_points' })

      expect(screen.getByRole('tablist')).toHaveAttribute('aria-label', 'Briefing type')
      expect(tab('Key Points')).toHaveAttribute('aria-selected', 'true')
      expect(tab('General Summary')).toHaveAttribute('aria-selected', 'false')
    })

    // One panel at a time is the whole point of tabs over the old accordion.
    it('renders exactly one panel, owned by the selected tab', () => {
      renderPane({ lenses: allGenerated, activeLens: 'action_items' })

      const panels = screen.getAllByRole('tabpanel')
      expect(panels).toHaveLength(1)

      const controlled = tab('Action Items').getAttribute('aria-controls')
      expect(controlled).toBeTruthy()
      expect(panels[0]).toHaveAttribute('id', controlled)
    })

    it('keeps the old #briefing-section-<type> anchor on the open panel', () => {
      const { container } = renderPane({ lenses: allGenerated, activeLens: 'q_and_a' })
      expect(container.querySelector('#briefing-section-q_and_a')).toBeInTheDocument()
      expect(container.querySelector('#briefing-section-general')).not.toBeInTheDocument()
    })

    it('moves between tabs with the arrow keys', async () => {
      const user = userEvent.setup()
      const onActiveLensChange = vi.fn()
      renderPane({ lenses: allGenerated, activeLens: 'general', onActiveLensChange })

      await user.click(tab('General Summary'))
      await user.keyboard('{ArrowRight}')
      expect(onActiveLensChange).toHaveBeenLastCalledWith('key_points')

      // Focus followed the arrow, so the next one steps on from there.
      await user.keyboard('{ArrowRight}')
      expect(onActiveLensChange).toHaveBeenLastCalledWith('action_items')

      // Left from the first tab wraps to the last.
      await user.click(tab('General Summary'))
      await user.keyboard('{ArrowLeft}')
      expect(onActiveLensChange).toHaveBeenLastCalledWith('q_and_a')
    })

    it('reports the tab the reader clicked instead of switching on its own', async () => {
      const user = userEvent.setup()
      const onActiveLensChange = vi.fn()
      renderPane({ lenses: allGenerated, activeLens: 'general', onActiveLensChange })

      await user.click(tab('Action Items'))

      expect(onActiveLensChange).toHaveBeenCalledWith('action_items')
      // Controlled: the pane shows what it was given until the owner says otherwise.
      expect(tab('General Summary')).toHaveAttribute('aria-selected', 'true')
    })

    it('falls back to the first lens when activeLens names none of them', () => {
      renderPane({
        lenses: toLenses([summary({ id: 's-general', summary_type: 'general' })]).slice(1),
        activeLens: 'general',
      })
      expect(tab('Key Points')).toHaveAttribute('aria-selected', 'true')
    })

    // A silent fallback would leave the shell (and the export panel's
    // "Briefing · {lens}" row) naming a type the band is not showing.
    it('reports the fallback so the owner agrees with the band', () => {
      const onActiveLensChange = vi.fn()
      renderPane({
        lenses: toLenses([summary({ id: 's-general', summary_type: 'general' })]).slice(1),
        activeLens: 'general',
        onActiveLensChange,
      })
      expect(onActiveLensChange).toHaveBeenCalledWith('key_points')
    })

    it('reports nothing when activeLens names a lens it was given', () => {
      const onActiveLensChange = vi.fn()
      renderPane({ lenses: allGenerated, activeLens: 'action_items', onActiveLensChange })
      expect(onActiveLensChange).not.toHaveBeenCalled()
    })
  })

  // The dot is the whole point of the tab row: it has to distinguish "never
  // asked for" from "running" from "done" without opening anything, and say so
  // to a reader who cannot see a ring or a colour.
  describe('status dots', () => {
    it('spells out every non-ready state for a screen reader', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', status: 'completed' }),
        summary({ id: 's-key', summary_type: 'key_points', status: 'pending' }),
        summary({ id: 's-actions', summary_type: 'action_items', status: 'failed' }),
      ])
      renderPane({ lenses })

      expect(tab('Key Points')).toHaveTextContent('Summarizing…')
      expect(tab('Action Items')).toHaveTextContent('Failed')
      expect(tab('Questions & Answers')).toHaveTextContent('Not generated')
    })

    // A dot on every tab is a dot that means nothing.
    it('says nothing on a ready tab', () => {
      renderPane({ lenses: allGenerated })
      expect(tab('General Summary')).toHaveTextContent('General Summary')
      expect(tab('General Summary')).not.toHaveTextContent('Ready')
    })

    it('reads as running the moment a generate is in flight, before its row exists', () => {
      renderPane({ lenses: toLenses([]), briefing: controls({ generatingType: 'q_and_a' }) })
      expect(tab('Questions & Answers')).toHaveTextContent('Summarizing…')
      expect(tab('General Summary')).toHaveTextContent('Not generated')
    })
  })

  it('shows the empty line for a session with no briefings', () => {
    renderPane({ lenses: toLenses([]) })

    expect(screen.getByText('No briefings for this session yet.')).toBeInTheDocument()
    expect(useSummaryDetailMock).not.toHaveBeenCalled()
  })

  // Briefings that appear without being asked for need an explanation, and
  // "no briefings yet" is the wrong one the moment four are on their way.
  it('accounts for generation this visit started on its own', () => {
    renderPane({ lenses: toLenses([]), briefing: controls({ autoStarted: true }) })

    expect(
      screen.getByText('Started automatically when you opened this session.')
    ).toBeInTheDocument()
    expect(screen.queryByText('No briefings for this session yet.')).not.toBeInTheDocument()
  })

  // The line explains an empty band. Once the briefings land it is explaining
  // nothing, and a permanent banner over four finished briefings is clutter.
  it('withdraws the auto-start line once a briefing lands', () => {
    renderPane({ lenses: allGenerated, briefing: controls({ autoStarted: true }) })

    expect(
      screen.queryByText('Started automatically when you opened this session.')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('No briefings for this session yet.')).not.toBeInTheDocument()
  })

  describe('per-type generation', () => {
    it('offers Generate, naming the type, on a lens that was never generated', async () => {
      const user = userEvent.setup()
      const { briefing } = renderPane({ lenses: toLenses([]), activeLens: 'action_items' })

      const button = screen.getByRole('button', { name: 'Generate Action Items' })
      await user.click(button)
      expect(briefing.generateType).toHaveBeenCalledWith('action_items')
    })

    it('passes the selected profile to a per-type generate', async () => {
      const user = userEvent.setup()
      const { briefing } = renderPane({
        lenses: toLenses([]),
        defaultSummaryProfile: 'legal',
        summaryProfilesEnabled: true,
      })

      await user.click(screen.getByRole('button', { name: 'Generate General Summary' }))
      expect(briefing.generateType).toHaveBeenCalledWith('general', 'legal')
    })

    it('offers no Generate on a lens that already has a briefing', () => {
      const lenses = toLenses([summary({ id: 's-general', summary_type: 'general' })])
      renderPane({ lenses, activeLens: 'general' })

      expect(screen.queryByRole('button', { name: /Generate General/ })).not.toBeInTheDocument()
    })

    it('disables Generate while one type is in flight', () => {
      renderPane({ lenses: toLenses([]), briefing: controls({ generatingType: 'general' }) })

      expect(screen.getByRole('button', { name: 'Generating…' })).toHaveAttribute(
        'aria-busy',
        'true'
      )
      expect(screen.getByRole('button', { name: 'Generating…' })).toBeDisabled()
    })

    // A tab band shows one panel, so a failure parked inside the panel would
    // vanish the moment the reader looked at another tab.
    it('keeps a per-type failure visible whichever tab is open', () => {
      renderPane({
        lenses: toLenses([]),
        activeLens: 'general',
        briefing: controls({
          generateTypeErrorType: 'q_and_a',
          generateTypeError: new Error('boom'),
        }),
      })

      const alerts = screen.getAllByRole('alert')
      expect(alerts).toHaveLength(1)
      expect(alerts[0]).toHaveTextContent("Couldn't generate this briefing. Please try again.")
    })
  })

  describe('generate all', () => {
    it('generates every missing type from the band header', async () => {
      const user = userEvent.setup()
      const { briefing } = renderPane({ lenses: toLenses([]) })

      await user.click(screen.getByRole('button', { name: 'Generate all' }))
      expect(briefing.generateAll).toHaveBeenCalledWith()
    })

    it('passes the selected profile to generate all', async () => {
      const user = userEvent.setup()
      const { briefing } = renderPane({
        lenses: toLenses([]),
        defaultSummaryProfile: 'investigation',
        summaryProfilesEnabled: true,
      })

      await user.click(screen.getByRole('button', { name: 'Generate all' }))
      expect(briefing.generateAll).toHaveBeenCalledWith('investigation')
    })

    // Nothing missing, nothing broken: the button would spend four billable
    // jobs re-enqueueing rows that already exist.
    it('is disabled once every lens is generated', () => {
      renderPane({ lenses: allGenerated })
      expect(screen.getByRole('button', { name: 'Generate all' })).toBeDisabled()
    })

    // The server's generate-all skips every existing row whose status is not
    // `pending` — a failed one included — so leaving the button live here would
    // be a click that silently does nothing. The failed lens keeps its own
    // Regenerate as the recovery path.
    it('is disabled when every type exists and one of them failed', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', status: 'completed' }),
        summary({ id: 's-key', summary_type: 'key_points', status: 'completed' }),
        summary({ id: 's-actions', summary_type: 'action_items', status: 'completed' }),
        summary({ id: 's-qa', summary_type: 'q_and_a', status: 'failed' }),
      ])
      renderPane({ lenses })

      expect(screen.getByRole('button', { name: 'Generate all' })).toBeDisabled()
      expect(tab('Questions & Answers')).toHaveTextContent('Failed')
    })

    // …but a type with no row at all still has something for it to do, even
    // when a different lens has failed.
    it('stays available while one type has never been generated', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', status: 'completed' }),
        summary({ id: 's-qa', summary_type: 'q_and_a', status: 'failed' }),
      ])
      renderPane({ lenses })

      expect(screen.getByRole('button', { name: 'Generate all' })).not.toBeDisabled()
    })

    it('shows a generate-all failure above the tabs', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Body.' }),
      ])
      renderPane({ lenses, briefing: controls({ error: new Error('boom') }) })

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't generate briefings. Please try again."
      )
    })
  })

  describe('panel content', () => {
    it('renders an embedded summary without fetching it', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Embedded body text.' }),
      ])
      renderPane({ lenses })

      expect(screen.getByText('Embedded body text.')).toBeInTheDocument()
      expect(useSummaryDetailMock).not.toHaveBeenCalled()
    })

    // Only the open lens is mounted, so three briefings the reader is not
    // looking at cost three requests they did not ask for.
    it('fetches only the open lens', () => {
      useSummaryDetailMock.mockReturnValue({
        data: summary({ id: 's-key', content: 'Fetched body text.' }),
        isLoading: false,
      })
      renderPane({ lenses: allGenerated, activeLens: 'key_points' })

      expect(useSummaryDetailMock).toHaveBeenCalledTimes(1)
      expect(useSummaryDetailMock).toHaveBeenCalledWith('s-key')
      expect(screen.getByText('Fetched body text.')).toBeInTheDocument()
    })

    it('passes citation seeks through to an embedded structured summary', async () => {
      const user = userEvent.setup()
      const onCitationSeek = vi.fn()
      const lenses = toLenses([
        summary({
          id: 's-general',
          summary_type: 'general',
          content: 'Canonical text.',
          structured_content: {
            schema_version: 'structured_summary_v1',
            matrix_language: 'en',
            paragraphs: [
              { id: 'g1', text: 'Structured text.', speaker: null, citation_ids: ['u1'] },
            ],
          },
          citations: [{ id: 'u1', start_seconds: 45 }],
        }),
      ])
      renderPane({ lenses, onCitationSeek })

      // Chips are hidden until the reader asks for them.
      expect(screen.queryByRole('button', { name: 'Citation u1, 00:45' })).not.toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Show sources' }))

      await user.click(screen.getByRole('button', { name: 'Citation u1, 00:45' }))

      expect(onCitationSeek).toHaveBeenCalledWith(45)
    })

    it('surfaces a fetch failure instead of an empty briefing', () => {
      useSummaryDetailMock.mockReturnValue({ data: undefined, isLoading: false, isError: true })

      renderPane({ lenses: toLenses([summary({ id: 's-general', summary_type: 'general' })]) })

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't load this briefing. Please try again."
      )
      expect(screen.queryByText('No summary content')).not.toBeInTheDocument()
    })

    it('explains an open lens that has nothing in it yet', () => {
      renderPane({ lenses: toLenses([]) })

      expect(screen.getByText("This briefing hasn't been generated yet.")).toBeInTheDocument()
    })
  })

  describe('provenance note', () => {
    it('dates a finished briefing and names its profile', () => {
      const lenses = toLenses([
        summary({
          id: 's-general',
          summary_type: 'general',
          content: 'Body.',
          summary_profile: 'legal',
          completed_at: '2026-09-03T09:00:00Z',
        }),
      ])
      renderPane({ lenses, summaryProfilesEnabled: true })

      expect(
        screen.getByText('Generated Sep 3, 2026 · Legal profile · Speaker names inferred by the model')
      ).toBeInTheDocument()
    })

    // A deployment with profiles switched off has no profile to name, and
    // naming one would advertise a control the reader does not have.
    it('drops the profile clause when profiles are disabled', () => {
      const lenses = toLenses([
        summary({
          id: 's-general',
          summary_type: 'general',
          content: 'Body.',
          summary_profile: 'legal',
          completed_at: '2026-09-03T09:00:00Z',
        }),
      ])
      renderPane({ lenses, summaryProfilesEnabled: false })

      expect(
        screen.getByText('Generated Sep 3, 2026 · Speaker names inferred by the model')
      ).toBeInTheDocument()
    })

    it('says nothing about a briefing that has not finished', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', status: 'pending' }),
      ])
      useSummaryDetailMock.mockReturnValue({
        data: summary({ id: 's-general', status: 'pending' }),
        isLoading: false,
      })
      renderPane({ lenses })

      expect(screen.queryByText(/Generated/)).not.toBeInTheDocument()
    })
  })

  describe('regeneration', () => {
    it('regenerates the open lens by summary id', async () => {
      const user = userEvent.setup()
      const lenses = toLenses([
        summary({ id: 's-key', summary_type: 'key_points', content: '- A point' }),
      ])
      const { briefing } = renderPane({ lenses, activeLens: 'key_points' })

      await user.click(screen.getByRole('button', { name: /Regenerate/ }))
      expect(briefing.regenerate).toHaveBeenCalledWith('s-key')
    })

    it('offers every profile once, above the tabs, and applies it', async () => {
      const user = userEvent.setup()
      const { briefing } = renderPane({
        lenses: allGenerated,
        activeLens: 'key_points',
        defaultSummaryProfile: 'legal',
        summaryProfilesEnabled: true,
      })

      const profileSelect = screen.getByRole('combobox', { name: 'Profile' })
      expect(profileSelect).toHaveTextContent('Legal')
      await user.click(profileSelect)
      for (const label of [
        'General Professional',
        'Legal',
        'Investment Analysis',
        'Journalism',
        'Negotiation',
        'Decision Committee',
        'Investigation',
      ]) {
        expect(screen.getByRole('option', { name: label })).toBeInTheDocument()
      }

      await user.click(screen.getByRole('option', { name: 'Investment Analysis' }))
      expect(screen.getByRole('combobox', { name: 'Profile' })).toHaveTextContent(
        'Investment Analysis'
      )

      // One panel, one Regenerate — the open lens's.
      const regenerate = screen.getByRole('button', { name: /Regenerate/ })
      await user.click(regenerate)

      expect(briefing.regenerate).toHaveBeenCalledOnce()
      expect(briefing.regenerate).toHaveBeenCalledWith('s-key', 'investment_analysis')
    })

    it('keeps the profile control hidden when professional profiles are disabled', () => {
      renderPane({ lenses: allGenerated, summaryProfilesEnabled: false })

      expect(screen.queryByRole('combobox', { name: 'Profile' })).not.toBeInTheDocument()
    })

    it('does not offer Regenerate for a lens that was never generated', () => {
      renderPane({ lenses: toLenses([]) })
      expect(screen.queryByRole('button', { name: /Regenerate/ })).not.toBeInTheDocument()
    })

    it('renders the settled regenerate error under the tabs, naming its type', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Body.' }),
      ])
      renderPane({
        lenses,
        briefing: controls({ regenerateErrorId: 's-general', regenerateError: new Error('boom') }),
      })

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't regenerate the General Summary briefing. Please try again."
      )
    })

    // Only the selected lens's body is mounted, so a notice parked inside it
    // was destroyed by the reader simply looking at another tab — taking the
    // only record that the request failed with it.
    it('keeps a regenerate failure on the page after the reader switches tabs', () => {
      const briefing = controls({
        regenerateErrorId: 's-key',
        regenerateError: new Error('boom'),
      })
      const { rerenderPane } = renderPane({
        lenses: allGenerated,
        activeLens: 'key_points',
        briefing,
      })

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't regenerate the Key Points briefing. Please try again."
      )

      rerenderPane({ activeLens: 'general' })

      // Still exactly one alert, still naming the lens that actually failed.
      expect(screen.getAllByRole('alert')).toHaveLength(1)
      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't regenerate the Key Points briefing. Please try again."
      )
    })

    it('does not show the regenerate error in a lens that did not fail', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Body.' }),
        summary({ id: 's-key', summary_type: 'key_points', content: '- A point' }),
      ])
      renderPane({
        lenses,
        briefing: controls({ regenerateErrorId: 's-missing', regenerateError: new Error('boom') }),
      })

      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })

    it('re-enables Regenerate once the attempt has settled with an error', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Body.' }),
      ])
      renderPane({
        lenses,
        briefing: controls({ regenerateErrorId: 's-general', regenerateError: new Error('boom') }),
      })

      expect(screen.getByRole('button', { name: /Regenerate/ })).not.toBeDisabled()
    })

    it('disables Regenerate while another lens is in flight', () => {
      const lenses = toLenses([
        summary({ id: 's-general', summary_type: 'general', content: 'Body.' }),
      ])
      renderPane({ lenses, briefing: controls({ regeneratingId: 's-other' }) })

      expect(screen.getByRole('button', { name: /Regenerate/ })).toBeDisabled()
    })
  })
})
