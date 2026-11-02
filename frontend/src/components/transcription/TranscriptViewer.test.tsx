import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TranscriptViewer } from './TranscriptViewer'
import type { Utterance } from '@/types/transcription'

const mockUtterances: Utterance[] = [
  {
    id: 1,
    speaker: 0,
    language: 'id',
    start: 0.0,
    end: 3.5,
    text: 'Hello, welcome to the meeting.',
    confidence: 0.95,
    words: [{ word: 'Hello', start: 0.0, end: 0.5, confidence: 0.98 }],
  },
  {
    id: 2,
    speaker: 1,
    language: 'id',
    start: 3.5,
    end: 7.0,
    text: 'Thank you for having me.',
    confidence: 0.72,
    words: [{ word: 'Thank', start: 3.5, end: 4.0, confidence: 0.7 }],
  },
  {
    id: 3,
    speaker: 0,
    language: 'id',
    start: 7.0,
    end: 12.0,
    text: 'Let us discuss the agenda.',
    confidence: 0.88,
    words: [{ word: 'Let', start: 7.0, end: 7.3, confidence: 0.9 }],
  },
]

type ViewerProps = Partial<React.ComponentProps<typeof TranscriptViewer>>

/** Follow is controlled by the reader shell, so every render supplies it. */
function renderViewer(props: ViewerProps = {}) {
  const onFollowingChange = props.onFollowingChange ?? vi.fn()
  const view = render(
    <TranscriptViewer
      utterances={mockUtterances}
      following
      {...props}
      onFollowingChange={onFollowingChange}
    />
  )
  const rerenderViewer = (next: ViewerProps = {}) =>
    view.rerender(
      <TranscriptViewer
        utterances={mockUtterances}
        following
        {...next}
        onFollowingChange={next.onFollowingChange ?? onFollowingChange}
      />
    )
  return { ...view, onFollowingChange, rerenderViewer }
}

describe('TranscriptViewer', () => {
  // The tray's pill and this one share a band on phones; side by side they
  // overlapped by 23 to 69 px at 375 px, depending on the language.
  it('keeps the Follow pill clear of the activity tray at phone and tablet widths', () => {
    renderViewer({ following: false })
    expect(screen.getByRole('button', { name: /follow/i })).toHaveClass(
      'md:max-lg:left-[calc(var(--app-sidebar-w)_+_1.5rem)]',
      'max-md:[body:has([data-activity-tray])_&]:bottom-[calc(var(--reader-dock-h,0rem)_+_5rem)]',
    )
  })

  beforeEach(() => {
    // jsdom has no layout engine; scrollTo is a stub that logs "not implemented".
    Element.prototype.scrollTo = vi.fn()
  })

  it('renders all utterances', () => {
    renderViewer()
    expect(screen.getByText('Hello, welcome to the meeting.')).toBeInTheDocument()
    expect(screen.getByText('Thank you for having me.')).toBeInTheDocument()
    expect(screen.getByText('Let us discuss the agenda.')).toBeInTheDocument()
  })

  it('renders default speaker labels', () => {
    renderViewer()
    expect(screen.getAllByText('Speaker 1')).toHaveLength(2)
    expect(screen.getByText('Speaker 2')).toBeInTheDocument()
  })

  it('renders custom speaker names from speakerMap', () => {
    renderViewer({ speakerMap: { '0': 'Alice', '1': 'Bob' } })
    expect(screen.getAllByText('Alice')).toHaveLength(2)
    expect(screen.getByText('Bob')).toBeInTheDocument()
  })

  it('falls back to "Speaker N" when a mapped name is empty or whitespace', () => {
    // Regression: a stored '' for one index must not render a blank label.
    renderViewer({ speakerMap: { '0': 'Alice', '1': '' } })
    expect(screen.getAllByText('Alice')).toHaveLength(2)
    expect(screen.getByText('Speaker 2')).toBeInTheDocument()
  })

  it('renders timestamps', () => {
    renderViewer()
    expect(screen.getByText('00:00')).toBeInTheDocument()
    expect(screen.getByText('00:03')).toBeInTheDocument()
    expect(screen.getByText('00:07')).toBeInTheDocument()
  })

  it('shows the low confidence flag in the meta rail', () => {
    renderViewer()
    // Second utterance has confidence 0.72 < 0.8
    const flag = screen.getByText('Low confidence')
    expect(flag).toBeInTheDocument()
    expect(flag).toHaveAttribute('title', 'Confidence: 72%')
    // It belongs to the rail, beside the speaker and the time — not the words.
    expect(flag.parentElement).toHaveTextContent('Speaker 200:03Low confidence')
  })

  it('renders an utterance without confidence (Speechmatics) without crashing or showing 0', () => {
    // Melia never returns confidence scores — the field is omitted entirely,
    // never a fabricated 0.
    const noConfidence: Utterance[] = [
      {
        id: 1,
        speaker: 0,
        start: 0.0,
        end: 3.5,
        text: 'Halo, selamat datang di rapat.',
        words: [{ word: 'Halo', start: 0.0, end: 0.5 }],
      },
    ]

    renderViewer({ utterances: noConfidence })

    expect(screen.getByText('Halo, selamat datang di rapat.')).toBeInTheDocument()
    expect(screen.queryByText('Low confidence')).not.toBeInTheDocument()
    expect(screen.queryByText('0')).not.toBeInTheDocument()
    expect(screen.queryByText('0%')).not.toBeInTheDocument()
  })

  it('calls onSeek when utterance is clicked', async () => {
    const onSeek = vi.fn()
    renderViewer({ onSeek })

    const user = userEvent.setup()
    await user.click(screen.getByText('Thank you for having me.'))
    expect(onSeek).toHaveBeenCalledWith(3.5)
  })

  // "Follow playback" is a switch the reader set. A click on a turn is a seek
  // and nothing else — re-engaging follow would also scroll them to the turn
  // that was playing before the seek, not the one they just clicked.
  it('does not re-engage follow when a turn is clicked with follow released', async () => {
    const scrollTo = vi.fn()
    Element.prototype.scrollTo = scrollTo
    const onSeek = vi.fn()
    const onFollowingChange = vi.fn()
    renderViewer({ onSeek, onFollowingChange, following: false, currentTime: 0.5 })

    const user = userEvent.setup()
    await user.click(screen.getByText('Let us discuss the agenda.'))

    expect(onSeek).toHaveBeenCalledWith(7.0)
    expect(onFollowingChange).not.toHaveBeenCalled()
    expect(scrollTo).not.toHaveBeenCalled()
  })

  it('shows empty state when no utterances', () => {
    renderViewer({ utterances: [] })
    expect(screen.getByText('No utterances available.')).toBeInTheDocument()
  })

  it('marks the active turn with the coral edge and the soft paper ground, borderless otherwise', () => {
    const { container } = renderViewer({ currentTime: 4.0 })
    // At currentTime=4.0, second utterance (start=3.5, end=7.0) should be active
    const buttons = container.querySelectorAll('button')
    expect(buttons[0]).not.toHaveClass('bg-paper-soft')
    expect(buttons[0].className).not.toMatch(/border-[lb]/)
    expect(buttons[1]).toHaveClass('bg-paper-soft')
    expect(buttons[1]).toHaveClass('before:bg-coral-mark')
    expect(buttons[0]).not.toHaveClass('before:bg-coral-mark')
    // Nothing shifts when a turn lights up: the same padding on both.
    expect(buttons[0]).toHaveClass('px-3')
    expect(buttons[1]).toHaveClass('px-3')
  })

  it('renders the turns on a rail beside the words', () => {
    const { container } = renderViewer()
    const row = container.querySelectorAll('[role="listitem"]')[0]
    expect(row?.querySelector('button')).toHaveClass('sm:grid-cols-[112px_minmax(0,1fr)]')
  })

  // The browser picks CJK glyph shapes and line breaking from `lang`, and a
  // screen reader switches voice; a turn without a provider language follows
  // the page instead.
  it('marks a turn with its provider language, and leaves it off when absent', () => {
    const utterances: Utterance[] = [
      mockUtterances[0],
      { ...mockUtterances[1], language: undefined },
    ]
    renderViewer({ utterances })
    expect(screen.getByText('Hello, welcome to the meeting.')).toHaveAttribute('lang', 'id')
    expect(screen.getByText('Thank you for having me.')).not.toHaveAttribute('lang')
  })

  it('marks a Mandarin turn as zh, which the CJK type rules match', () => {
    renderViewer({ utterances: [{ ...mockUtterances[0], language: 'cmn' }] })
    expect(screen.getByText('Hello, welcome to the meeting.')).toHaveAttribute('lang', 'zh')
  })

  // A `role="listitem"` on the button would replace the button role, and with it
  // the only cue that says a turn can be pressed to seek.
  it('keeps every turn a button inside its list item', () => {
    const { container } = renderViewer()
    const rows = container.querySelectorAll('[role="listitem"]')
    expect(rows).toHaveLength(3)
    for (const row of rows) {
      expect(row.tagName).toBe('DIV')
      expect(row.querySelector('button')).not.toBeNull()
    }
    expect(screen.getAllByRole('button')).toHaveLength(3)
  })

  // From lg the transcript scrolls in its own box; below lg there is no box
  // (no height cap, no overflow), so the page is the only scroll on a phone.
  it('renders transcript in a scroll box from lg only', () => {
    const { container } = renderViewer()
    const scrollContainer = container.querySelector('[data-testid="transcript-scroll-container"]')
    expect(scrollContainer).toBeInTheDocument()
    expect(scrollContainer).toHaveClass('lg:overflow-y-auto')
    expect(scrollContainer).toHaveClass('lg:overscroll-contain')
    expect(scrollContainer).toHaveClass(
      'lg:max-h-[calc(100vh_-_10rem_-_var(--reader-nav-h,0rem)_-_var(--reader-dock-h,0rem))]'
    )
    expect(scrollContainer).not.toHaveClass('max-h-[60vh]')
    expect(scrollContainer).not.toHaveClass('overflow-y-auto')
    // The gutter is reserved so the words never sit under a hover scrollbar.
    expect(scrollContainer).toHaveClass('lg:[scrollbar-gutter:stable]')
  })

  describe('controlled follow', () => {
    it('hides the Follow pill while following', () => {
      renderViewer({ currentTime: 4.0, following: true })
      expect(screen.queryByRole('button', { name: /follow playback/i })).not.toBeInTheDocument()
    })

    it('shows the Follow pill once following is released', () => {
      renderViewer({ currentTime: 4.0, following: false })
      expect(screen.getByRole('button', { name: /follow playback/i })).toBeInTheDocument()
    })

    // The viewer reports; it never flips the switch itself. A viewer that owned
    // the state would disagree with the toolbar switch the moment either moved.
    it('reports a release when the reader scrolls away', () => {
      const onFollowingChange = vi.fn()
      const { container } = renderViewer({ currentTime: 4.0, onFollowingChange })
      const scrollContainer = container.querySelector('[data-testid="transcript-scroll-container"]')
      expect(scrollContainer).not.toBeNull()

      act(() => {
        Object.defineProperty(scrollContainer!, 'scrollTop', {
          value: 100,
          writable: true,
          configurable: true,
        })
        scrollContainer!.dispatchEvent(new Event('scroll'))
      })

      expect(onFollowingChange).toHaveBeenCalledWith(false)
      // Still shown as following: only the owner can change that.
      expect(screen.queryByRole('button', { name: /follow playback/i })).not.toBeInTheDocument()
    })

    it('reports re-engagement when the Follow pill is pressed', async () => {
      const onFollowingChange = vi.fn()
      renderViewer({ currentTime: 4.0, following: false, onFollowingChange })

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /follow playback/i }))

      expect(onFollowingChange).toHaveBeenCalledWith(true)
    })
  })

  describe('find in transcript', () => {
    it('marks matching text and dims non-matching rows', () => {
      const { container } = renderViewer({ searchQuery: 'the' })

      const marks = container.querySelectorAll('mark')
      // "the meeting" and "the agenda" — one mark each, row 2 has no "the".
      expect(marks).toHaveLength(2)
      expect(marks[0]).toHaveTextContent('the')
      expect(marks[0]?.className).toContain('bg-coral/20')

      const rows = container.querySelectorAll('[role="listitem"]')
      expect(rows[0]).toHaveAttribute('data-match', 'true')
      expect(rows[1]).toHaveAttribute('data-match', 'false')
      expect(rows[2]).toHaveAttribute('data-match', 'true')
      expect(rows[1]?.querySelector('button')).toHaveClass('opacity-40')
      expect(rows[0]?.querySelector('button')).not.toHaveClass('opacity-40')
    })

    it('never removes rows while a query is active', () => {
      const { container } = renderViewer({ searchQuery: 'zzz-no-hits' })
      // Index addressing (auto-scroll refs, keys) breaks if rows disappear.
      expect(container.querySelectorAll('[role="listitem"]')).toHaveLength(3)
      expect(container.querySelectorAll('mark')).toHaveLength(0)
    })

    it('matches case-insensitively', () => {
      const { container } = renderViewer({ searchQuery: 'HELLO' })
      const marks = container.querySelectorAll('mark')
      expect(marks).toHaveLength(1)
      expect(marks[0]).toHaveTextContent('Hello')
    })

    it('reports match indices to onMatchesChange', () => {
      const onMatchesChange = vi.fn()
      const { rerenderViewer } = renderViewer({ searchQuery: 'the', onMatchesChange })
      expect(onMatchesChange).toHaveBeenLastCalledWith([0, 2])

      rerenderViewer({ searchQuery: '', onMatchesChange })
      expect(onMatchesChange).toHaveBeenLastCalledWith([])
    })

    // The reader shell resets `activeMatch` to -1 on every match-list change, so
    // typing would otherwise look exactly like stepping to the first match — and
    // silently switch "Follow playback" off, with nothing to switch it back on.
    it('does not jump or release follow while the query is being typed', () => {
      const scrollTo = vi.fn()
      Element.prototype.scrollTo = scrollTo
      const onFollowingChange = vi.fn()

      const { rerenderViewer, container } = renderViewer({
        searchQuery: 't',
        activeMatch: -1,
        onFollowingChange,
      })
      expect(scrollTo).not.toHaveBeenCalled()
      expect(onFollowingChange).not.toHaveBeenCalledWith(false)

      // Next character: still typing, `activeMatch` still reset.
      rerenderViewer({ searchQuery: 'th', activeMatch: -1, onFollowingChange })
      expect(scrollTo).not.toHaveBeenCalled()
      expect(onFollowingChange).not.toHaveBeenCalledWith(false)

      // The first match is still the one flagged for the reader to see.
      expect(container.querySelectorAll('[role="listitem"]')[0]).toHaveAttribute(
        'data-active-match',
        'true'
      )
    })

    it('jumps to the first match on the first step', () => {
      const scrollTo = vi.fn()
      Element.prototype.scrollTo = scrollTo
      const onFollowingChange = vi.fn()

      const { rerenderViewer } = renderViewer({ searchQuery: 'the', activeMatch: -1, onFollowingChange })
      expect(scrollTo).not.toHaveBeenCalled()

      rerenderViewer({ searchQuery: 'the', activeMatch: 0, onFollowingChange })
      expect(scrollTo).toHaveBeenCalledTimes(1)
      expect(onFollowingChange).toHaveBeenCalledWith(false)
    })

    it('does not jump when a stepped match is reset by the query changing', () => {
      const scrollTo = vi.fn()
      Element.prototype.scrollTo = scrollTo
      const { rerenderViewer } = renderViewer({ searchQuery: 'the', activeMatch: 1 })
      rerenderViewer({ searchQuery: 'th', activeMatch: 1 })
      rerenderViewer({ searchQuery: 'th', activeMatch: -1 })
      expect(scrollTo).not.toHaveBeenCalled()
    })

    it('scrolls to the segment named by activeMatch and releases follow when stepping', () => {
      const scrollTo = vi.fn()
      Element.prototype.scrollTo = scrollTo
      const onFollowingChange = vi.fn()

      const { rerenderViewer, container } = renderViewer({
        searchQuery: 'the',
        activeMatch: 0,
        onFollowingChange,
      })

      rerenderViewer({ searchQuery: 'the', activeMatch: 1, onFollowingChange })
      expect(scrollTo).toHaveBeenCalled()
      expect(onFollowingChange).toHaveBeenCalledWith(false)
      expect(container.querySelectorAll('[role="listitem"]')[2]).toHaveAttribute(
        'data-active-match',
        'true'
      )

      // And wrapping back to the first match is a step too.
      scrollTo.mockClear()
      rerenderViewer({ searchQuery: 'the', activeMatch: 0, onFollowingChange })
      expect(scrollTo).toHaveBeenCalled()
    })

    it('leaves follow alone when the query is cleared', () => {
      const onFollowingChange = vi.fn()
      const { rerenderViewer } = renderViewer({
        searchQuery: 'the',
        activeMatch: 0,
        onFollowingChange,
      })
      rerenderViewer({ searchQuery: '', activeMatch: 0, onFollowingChange })
      expect(onFollowingChange).not.toHaveBeenCalled()
    })

    // `İ`.toLowerCase() is two code units, so indices from the lowercased copy
    // do not address the original: slicing by them mangled the words.
    it('renders text whose lowercasing changes length without mangling it', () => {
      const turkish: Utterance[] = [
        {
          id: 1,
          speaker: 0,
          start: 0,
          end: 3,
          text: 'İstanbul meeting notes',
          words: [{ word: 'İstanbul', start: 0, end: 1 }],
        },
      ]
      const { container } = renderViewer({ utterances: turkish, searchQuery: 'meeting' })

      // The row is still a match (and still navigable) — it just renders
      // unmarked rather than marking "eeting " one character off.
      expect(container.querySelector('[role="listitem"]')).toHaveAttribute('data-match', 'true')
      expect(container.querySelectorAll('mark')).toHaveLength(0)
      expect(container.querySelector('p')?.textContent).toBe('İstanbul meeting notes')
    })

    it('keeps click-to-seek working while a query is active', async () => {
      const onSeek = vi.fn()
      renderViewer({ onSeek, searchQuery: 'the' })

      const user = userEvent.setup()
      // Row 2 is dimmed (no match) but must still be clickable.
      await user.click(screen.getByText('Thank you for having me.'))
      expect(onSeek).toHaveBeenCalledWith(3.5)
    })

    it('keeps the active-playback highlight while a query is active', () => {
      const { container } = renderViewer({ currentTime: 4.0, searchQuery: 'the' })
      const rows = container.querySelectorAll('[role="listitem"]')
      // Row 2 is the playing segment AND a non-match: dimmed, still highlighted.
      expect(rows[1]?.querySelector('button')).toHaveClass('bg-paper-soft')
      expect(rows[1]?.querySelector('button')).toHaveClass('opacity-40')
    })
  })

  describe('script layout', () => {
    const run: Utterance[] = [
      { ...mockUtterances[0], speaker: 0, language: 'id' },
      { ...mockUtterances[1], speaker: 0, language: 'id' },
      { ...mockUtterances[2], speaker: 1, language: 'en' },
    ]

    it('shows the name and disc only when the speaker changes, with air above the change', () => {
      const { container } = renderViewer({ utterances: run, speakerMap: { '0': 'Ana Vidal' } })
      const rows = container.querySelectorAll('[role="listitem"]')
      // The follow-on turn keeps its name for a screen reader only.
      expect(rows[1]?.querySelector('.sr-only')).toHaveTextContent('Ana Vidal')
      expect(rows[1]?.querySelector('[data-testid="speaker-disc"]')).toBeNull()
      expect(rows[0]?.querySelector('[data-testid="speaker-disc"]')).toHaveTextContent('A')
      // Unnamed: the disc carries the speaker number.
      expect(rows[2]?.querySelector('[data-testid="speaker-disc"]')).toHaveTextContent('2')
      expect(rows[2]).toHaveClass('pt-3')
      expect(rows[1]).not.toHaveClass('pt-3')
      expect(rows[0]).not.toHaveClass('pt-3')
    })

    it('colours the disc by speaker number and never the text', () => {
      const { container } = renderViewer({ utterances: run })
      const discs = container.querySelectorAll('[data-testid="speaker-disc"]')
      expect(discs[0]).toHaveClass('bg-peach')
      expect(discs[1]).toHaveClass('bg-mint')
      expect(discs[0]).toHaveAttribute('aria-hidden', 'true')
      expect(screen.getAllByText('Speaker 1')[0]).toHaveClass('text-foreground')
    })

    it('labels a language switch in codes, with the switch spelled out for a screen reader', () => {
      renderViewer({ utterances: run })
      const label = screen.getByText('ID → EN')
      expect(label).toHaveAttribute('aria-hidden', 'true')
      expect(label.parentElement).toHaveClass('text-info')
      expect(
        screen.getByText('Detected language changes from Indonesian to English'),
      ).toHaveClass('sr-only')
      expect(screen.getAllByText(/→/)).toHaveLength(1)
    })
  })

  describe('citation landing', () => {
    it('rings the turn the citation lands on and brings it on screen once per landing', () => {
      const scrollTo = vi.fn()
      Element.prototype.scrollTo = scrollTo
      const { container, rerenderViewer } = renderViewer({ landing: { time: 4, nonce: 1 } })
      const rows = container.querySelectorAll('[role="listitem"]')
      expect(rows[1]?.querySelector('.citation-landing')).not.toBeNull()
      expect(container.querySelectorAll('.citation-landing')).toHaveLength(1)
      expect(scrollTo).toHaveBeenCalledTimes(1)

      // Same landing re-rendered: no second scroll.
      rerenderViewer({ landing: { time: 4, nonce: 1 } })
      expect(scrollTo).toHaveBeenCalledTimes(1)

      // Cleared by the owner after 1.2 s: the ring goes.
      rerenderViewer({ landing: null })
      expect(container.querySelector('.citation-landing')).toBeNull()
    })
  })
})
