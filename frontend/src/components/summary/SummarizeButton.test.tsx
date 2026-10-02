import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SummarizeButton } from './SummarizeButton'

const mockMutate = vi.fn()
const mockUseCreateSummary = vi.fn()

vi.mock('@/hooks/useSummary', () => ({
  useCreateSummary: () => mockUseCreateSummary(),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { toast } from '@/lib/toast'

describe('SummarizeButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseCreateSummary.mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    })
  })

  it('renders button when transcription completed', () => {
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    expect(
      screen.getByRole('button', { name: /summarize/i })
    ).toBeInTheDocument()
  })

  it('hidden when transcription not completed', () => {
    const { container } = render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="pending" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('opens dialog on click', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    expect(screen.getByText('Generate Summary')).toBeInTheDocument()
  })

  it('submits with selected type', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    // Open dialog
    await user.click(screen.getByRole('button', { name: /summarize/i }))

    // Open the summary type select
    await user.click(screen.getByRole('combobox'))

    // Select "Key Points"
    await user.click(screen.getByRole('option', { name: /key points/i }))

    // Submit
    await user.click(screen.getByRole('button', { name: /start summarization/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { transcription_id: 'tx-1', summary_type: 'key_points' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('disables during submission', async () => {
    mockUseCreateSummary.mockReturnValue({
      mutate: mockMutate,
      isPending: true,
    })
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    expect(screen.getByText('Generating...')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /generating/i })
    ).toBeDisabled()
  })

  it('closes dialog on success and shows toast', async () => {
    const onSuccess = vi.fn()
    mockMutate.mockImplementation(
      (_data: unknown, opts: { onSuccess: () => void }) => {
        opts.onSuccess()
      }
    )
    const user = userEvent.setup()
    render(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        onSuccess={onSuccess}
      />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('button', { name: /start summarization/i }))
    expect(onSuccess).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('Summarization started')
  })

  it('shows error toast on failure', async () => {
    mockMutate.mockImplementation(
      (_data: unknown, opts: { onError: (err: Error) => void }) => {
        opts.onError(new Error('server error'))
      }
    )
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('button', { name: /start summarization/i }))
    expect(toast.error).toHaveBeenCalledWith(
      'Failed to start summarization. Please try again.'
    )
  })

  it('uses localized fallback for unknown server messages', async () => {
    const apiError = Object.assign(new Error('Bad Request'), {
      data: { message: 'transcription content not available' },
    })
    mockMutate.mockImplementation(
      (_data: unknown, opts: { onError: (err: Error) => void }) => {
        opts.onError(apiError)
      }
    )
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('button', { name: /start summarization/i }))
    expect(toast.error).toHaveBeenCalledWith('Failed to start summarization. Please try again.')
  })

  it('shows all 4 type options', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))

    // Open the select dropdown
    await user.click(screen.getByRole('combobox'))

    expect(screen.getByRole('option', { name: /general summary/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /key points/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /action items/i })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /questions & answers/i })).toBeInTheDocument()
  })

  it('submits Questions & Answers when selected', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /questions & answers/i }))
    await user.click(screen.getByRole('button', { name: /start summarization/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { transcription_id: 'tx-1', summary_type: 'q_and_a' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('defaults to first available type when general exists', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        existingSummaryTypes={['general']}
      />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))

    // Submit without changing the select — default should be key_points
    await user.click(screen.getByRole('button', { name: /start summarization/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { transcription_id: 'tx-1', summary_type: 'key_points' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('returns null when all types already exist', () => {
    const { container } = render(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        existingSummaryTypes={['general', 'key_points', 'action_items', 'q_and_a']}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('disables already-existing types', async () => {
    const user = userEvent.setup()
    render(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        existingSummaryTypes={['general', 'action_items', 'q_and_a']}
      />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))

    // Open the select dropdown
    await user.click(screen.getByRole('combobox'))

    const generalOption = screen.getByRole('option', { name: /general summary/i })
    const keyPointsOption = screen.getByRole('option', { name: /key points/i })
    const actionItemsOption = screen.getByRole('option', { name: /action items/i })
    const questionsOption = screen.getByRole('option', { name: /questions & answers/i })

    expect(generalOption).toHaveAttribute('data-disabled')
    expect(keyPointsOption).not.toHaveAttribute('data-disabled')
    expect(actionItemsOption).toHaveAttribute('data-disabled')
    expect(questionsOption).toHaveAttribute('data-disabled')
  })

  it('resets selected type when existing summaries change', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /questions & answers/i }))

    rerender(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        existingSummaryTypes={['q_and_a']}
      />
    )
    await user.click(screen.getByRole('button', { name: /start summarization/i }))

    expect(mockMutate).toHaveBeenCalledWith(
      { transcription_id: 'tx-1', summary_type: 'general' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('does not submit a summary type that became unavailable', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <SummarizeButton transcriptionId="tx-1" transcriptionStatus="completed" />
    )
    await user.click(screen.getByRole('button', { name: /summarize/i }))
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /questions & answers/i }))

    rerender(
      <SummarizeButton
        transcriptionId="tx-1"
        transcriptionStatus="completed"
        existingSummaryTypes={['general', 'key_points', 'action_items', 'q_and_a']}
      />
    )

    expect(mockMutate).not.toHaveBeenCalled()
  })
})
