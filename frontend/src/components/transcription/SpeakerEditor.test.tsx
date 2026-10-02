import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpeakerEditor } from './SpeakerEditor'
import type { SpeakerSuggestion } from '@/types/transcription'

const mockUpdateMutate = vi.fn()
const mockGenerateMutate = vi.fn()
const mockDismissMutate = vi.fn()
const mockUseUpdateSpeakers = vi.fn()
const mockUseGenerate = vi.fn()
const mockUseDismiss = vi.fn()

vi.mock('@/hooks/useTranscription', () => ({
  useUpdateSpeakers: (...args: unknown[]) => mockUseUpdateSpeakers(...args),
  useGenerateSpeakerSuggestions: (...args: unknown[]) => mockUseGenerate(...args),
  useDismissSpeakerSuggestion: (...args: unknown[]) => mockUseDismiss(...args),
}))

vi.mock('@/lib/toast', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

function suggestion(name: string, confidence: SpeakerSuggestion['confidence']): SpeakerSuggestion {
  return { name, evidence: `${name} introduced themselves`, confidence }
}

describe('SpeakerEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseUpdateSpeakers.mockReturnValue({ mutate: mockUpdateMutate, isPending: false })
    mockUseGenerate.mockReturnValue({ mutate: mockGenerateMutate, isPending: false })
    mockUseDismiss.mockReturnValue({ mutate: mockDismissMutate, isPending: false })
  })

  it('renders a labelled input for each speaker', () => {
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={3} />)
    expect(screen.getByLabelText('Speaker 1')).toBeInTheDocument()
    expect(screen.getByLabelText('Speaker 2')).toBeInTheDocument()
    expect(screen.getByLabelText('Speaker 3')).toBeInTheDocument()
  })

  // --- Row states ---

  it('renders the no-data state with an empty value and a "Speaker N" placeholder', () => {
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={2} />)
    const input0 = screen.getByLabelText('Speaker 1')
    expect(input0).toHaveValue('')
    expect(input0).toHaveAttribute('placeholder', 'Speaker 1')
    expect(screen.queryByRole('button', { name: /Accept suggestion/ })).not.toBeInTheDocument()
  })

  it('renders the confirmed state with a plain name and no AI affordances', () => {
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{ '0': 'Alice' }}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Suggested', 'high') }}
      />
    )
    expect(screen.getByLabelText('Speaker 1')).toHaveValue('Alice')
    expect(screen.queryByText('Suggested')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Accept/ })).not.toBeInTheDocument()
  })

  it('renders the suggested name as a placeholder plus a chip with Accept and Dismiss', () => {
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Dr. Wijaya', 'high') }}
      />
    )
    const input = screen.getByLabelText('Speaker 1')
    // The suggestion is a placeholder so the first keystroke replaces it cleanly.
    expect(input).toHaveValue('')
    expect(input).toHaveAttribute('placeholder', 'Dr. Wijaya')
    // The suggestion also reads as a chip, so the name is legible without
    // clicking into the field to reveal the placeholder.
    expect(screen.getByText('Dr. Wijaya')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Accept suggestion Dr. Wijaya for speaker 1/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Dismiss suggestion for speaker 1' })
    ).toBeInTheDocument()
  })

  it('typing into a suggested row replaces the suggestion instead of appending to it', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Dr. Wijaya', 'high') }}
      />
    )
    const input = screen.getByLabelText('Speaker 1')
    await user.type(input, 'A')
    // The value is the typed char alone — never 'Dr. Wijaya A'.
    expect(input).toHaveValue('A')
    // The row is now a dirty edit, so blurring PATCHes the typed value.
    await user.tab()
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'A' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  // --- Low-confidence toggle ---

  it('hides low-confidence suggestions by default and reveals them after toggling', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Maybe Sutanto', 'low') }}
      />
    )
    // Hidden -> renders as no-data (empty, placeholder, no chip).
    expect(screen.getByLabelText('Speaker 1')).toHaveValue('')
    expect(screen.queryByText('Maybe Sutanto')).not.toBeInTheDocument()

    await user.click(screen.getByLabelText('Show low-confidence suggestions'))

    const input = screen.getByLabelText('Speaker 1')
    expect(input).toHaveValue('')
    expect(input).toHaveAttribute('placeholder', 'Maybe Sutanto')
    expect(screen.getByText('Maybe Sutanto')).toBeInTheDocument()
  })

  it('only renders the low-confidence toggle when a low suggestion exists', () => {
    const { rerender } = render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Alice', 'high') }}
      />
    )
    expect(screen.queryByLabelText('Show low-confidence suggestions')).not.toBeInTheDocument()

    rerender(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Bob', 'low') }}
      />
    )
    expect(screen.getByLabelText('Show low-confidence suggestions')).toBeInTheDocument()
  })

  // --- Actions ---

  it('accepts a suggestion by PATCHing the map with the accepted name', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={2}
        suggestedSpeakerMap={{ '0': suggestion('Dr. Wijaya', 'high') }}
      />
    )
    await user.click(screen.getByRole('button', { name: /Accept suggestion Dr. Wijaya/ }))
    // The untouched second row is empty, so it is OMITTED — only real names are sent.
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'Dr. Wijaya' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it("accepting a suggestion preserves another row's already-confirmed name", async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{ '1': 'Bob' }}
        speakerCount={2}
        suggestedSpeakerMap={{ '0': suggestion('Dr. Wijaya', 'high') }}
      />
    )
    await user.click(screen.getByRole('button', { name: /Accept suggestion Dr. Wijaya/ }))
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'Dr. Wijaya', '1': 'Bob' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('dismisses a suggestion with the correct index', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={2}
        suggestedSpeakerMap={{ '1': suggestion('Bob', 'high') }}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Dismiss suggestion for speaker 2' }))
    expect(mockDismissMutate).toHaveBeenCalledWith(
      '1',
      expect.objectContaining({ onError: expect.any(Function), onSettled: expect.any(Function) })
    )
    expect(mockUpdateMutate).not.toHaveBeenCalled()
  })

  it('PATCHes the typed value on blur', async () => {
    const user = userEvent.setup()
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={2} />)
    const input = screen.getByLabelText('Speaker 1')
    await user.type(input, 'Alice')
    await user.tab()
    // The untouched second row is empty, so it is OMITTED from the committed map.
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'Alice' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('PATCHes the typed value on Enter', async () => {
    const user = userEvent.setup()
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={2} />)
    const input = screen.getByLabelText('Speaker 1')
    await user.type(input, 'Charlie{Enter}')
    // The untouched second row is empty, so it is OMITTED from the committed map.
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'Charlie' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('does not PATCH when an untouched row is blurred', async () => {
    const user = userEvent.setup()
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{ '0': 'Alice' }} speakerCount={1} />)
    const input = screen.getByLabelText('Speaker 1')
    await user.click(input)
    await user.tab()
    expect(mockUpdateMutate).not.toHaveBeenCalled()
  })

  it('omits empty rows so a single named speaker is never sent with blank siblings', async () => {
    const user = userEvent.setup()
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={3} />)
    const input = screen.getByLabelText('Speaker 2')
    await user.type(input, 'Alice{Enter}')
    // Only the named index is sent — blank siblings would render as blank labels.
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '1': 'Alice' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('omits a previously-confirmed name when cleared to empty (un-name)', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{ '0': 'Alice', '1': 'Bob' }}
        speakerCount={2}
      />
    )
    const input = screen.getByLabelText('Speaker 1')
    await user.clear(input)
    await user.tab()
    // Clearing index 0 drops it; the whole-map replace keeps only Bob.
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '1': 'Bob' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  // --- Dirty-row protection ---

  it('does not overwrite a row the user is editing when props change', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={1} />
    )
    const input = screen.getByLabelText('Speaker 1')
    await user.type(input, 'My Edit')

    // A refetch delivers a suggestion for the same row mid-edit.
    rerender(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={1}
        suggestedSpeakerMap={{ '0': suggestion('Other Name', 'high') }}
      />
    )
    expect(screen.getByLabelText('Speaker 1')).toHaveValue('My Edit')
  })

  // --- Auto-generate ---

  it('auto-generates suggestions exactly once when suggestions_generated is false', () => {
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={2}
        suggestionsGenerated={false}
      />
    )
    expect(mockGenerateMutate).toHaveBeenCalledTimes(1)
  })

  it('does not auto-generate when suggestions_generated is true', () => {
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={2}
        suggestionsGenerated={true}
      />
    )
    expect(mockGenerateMutate).not.toHaveBeenCalled()
  })

  it('does not auto-generate when suggestions already exist', () => {
    render(
      <SpeakerEditor
        transcriptionId="tx-1"
        speakerMap={{}}
        speakerCount={2}
        suggestionsGenerated={false}
        suggestedSpeakerMap={{ '0': suggestion('Alice', 'high') }}
      />
    )
    expect(mockGenerateMutate).not.toHaveBeenCalled()
  })

  it('shows the "Finding names…" indicator while generation is pending', () => {
    mockUseGenerate.mockReturnValue({ mutate: mockGenerateMutate, isPending: true })
    render(
      <SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={1} suggestionsGenerated />
    )
    expect(screen.getByText('Finding names…')).toBeInTheDocument()
  })

  // --- Row folding ---

  it('keeps four or fewer speakers unfolded', () => {
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={4} />)
    expect(screen.getByLabelText('Speaker 4')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Show all/ })).not.toBeInTheDocument()
  })

  // Ten inputs would push the forensics panel off the column; the first four
  // are the ones a reader names before anything else.
  it('folds the rest behind "Show all" and reveals them on click', async () => {
    const user = userEvent.setup()
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={6} />)

    expect(screen.getByLabelText('Speaker 4')).toBeInTheDocument()
    expect(screen.queryByLabelText('Speaker 5')).not.toBeInTheDocument()

    const toggle = screen.getByRole('button', { name: 'Show all 6 speakers' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    await user.click(toggle)

    expect(screen.getByLabelText('Speaker 5')).toBeInTheDocument()
    expect(screen.getByLabelText('Speaker 6')).toBeInTheDocument()
    const collapse = screen.getByRole('button', { name: 'Show fewer' })
    expect(collapse).toHaveAttribute('aria-expanded', 'true')

    await user.click(collapse)
    expect(screen.queryByLabelText('Speaker 5')).not.toBeInTheDocument()
  })

  // A folded row is not sent as a blank: the commit reads every index, not
  // just the ones on screen.
  it('keeps a folded confirmed name when a visible row is committed', async () => {
    const user = userEvent.setup()
    render(
      <SpeakerEditor transcriptionId="tx-1" speakerMap={{ '5': 'Zara' }} speakerCount={6} />
    )
    await user.type(screen.getByLabelText('Speaker 1'), 'Alice{Enter}')
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { '0': 'Alice', '5': 'Zara' },
      expect.objectContaining({ onError: expect.any(Function) })
    )
  })

  it('titles the panel "Speakers"', () => {
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={1} />)
    expect(screen.getByText('Speakers')).toBeInTheDocument()
  })

  it('shows the saving indicator when the update mutation is pending', () => {
    mockUseUpdateSpeakers.mockReturnValue({ mutate: mockUpdateMutate, isPending: true })
    render(<SpeakerEditor transcriptionId="tx-1" speakerMap={{}} speakerCount={1} />)
    expect(screen.getByLabelText('Saving')).toBeInTheDocument()
  })
})
