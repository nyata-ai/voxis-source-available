import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TranscriptionSection } from './TranscriptionSection'

const mockMutate = vi.fn()

vi.mock('@/hooks/useSettings', () => ({
  usePreferences: vi.fn(),
  useUpdatePreferences: vi.fn(() => ({
    mutate: mockMutate,
  })),
}))

import { usePreferences } from '@/hooks/useSettings'

describe('TranscriptionSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['auto'],
        default_diarization: true,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.0,
        high_stakes_summaries: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)
  })

  it('renders section heading', () => {
    render(<TranscriptionSection />)
    expect(screen.getByText('Transcription')).toBeInTheDocument()
    expect(screen.getByText('Default transcription settings')).toBeInTheDocument()
  })

  it('renders language label and auto-detect button', () => {
    render(<TranscriptionSection />)
    expect(screen.getByText('Language')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /auto-detect/i })).toBeInTheDocument()
  })

  it('renders diarization switch', () => {
    render(<TranscriptionSection />)
    expect(screen.getByText('Speaker Diarization')).toBeInTheDocument()
    const toggle = screen.getByRole('switch')
    expect(toggle).toBeInTheDocument()
  })

  it('diarization switch reflects current preference value', () => {
    render(<TranscriptionSection />)
    const toggle = screen.getByRole('switch')
    // checked when default_diarization is true
    expect(toggle).toHaveAttribute('data-state', 'checked')
  })

  it('diarization switch is unchecked when preference is false', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['auto'],
        default_diarization: false,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.0,
        high_stakes_summaries: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<TranscriptionSection />)
    const toggle = screen.getByRole('switch')
    expect(toggle).toHaveAttribute('data-state', 'unchecked')
  })

  it('toggling diarization calls updatePreferences', async () => {
    const user = userEvent.setup()
    render(<TranscriptionSection />)

    await user.click(screen.getByRole('switch'))
    expect(mockMutate).toHaveBeenCalledWith({ default_diarization: false })
  })

  it('clicking a language chip calls updatePreferences with that language', async () => {
    const user = userEvent.setup()
    render(<TranscriptionSection />)

    // Click "English" to switch from auto-detect to English
    await user.click(screen.getByRole('button', { name: 'English' }))
    expect(mockMutate).toHaveBeenCalledWith({ default_languages: ['en'] })
  })

  it('clicking auto-detect calls updatePreferences with auto', async () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['en'],
        default_diarization: true,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.0,
        high_stakes_summaries: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    const user = userEvent.setup()
    render(<TranscriptionSection />)

    await user.click(screen.getByRole('button', { name: /auto-detect/i }))
    expect(mockMutate).toHaveBeenCalledWith({ default_languages: ['auto'] })
  })

  it('shows selected count when not auto-detect', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['en', 'id'],
        default_diarization: true,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.0,
        high_stakes_summaries: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<TranscriptionSection />)
    expect(screen.getByText('2/5 selected')).toBeInTheDocument()
  })

  it('shows loading skeleton when preferences are loading', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<TranscriptionSection />)
    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThan(0)
  })
})
