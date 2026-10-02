import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SummarySection } from './SummarySection'
import type { UserPreferences } from '@/types/settings'

const mockMutate = vi.fn()

vi.mock('@/hooks/useSettings', () => ({
  usePreferences: vi.fn(),
  useUpdatePreferences: vi.fn(() => ({
    mutate: mockMutate,
  })),
}))

vi.mock('@/hooks/useFeatures', () => ({
  useFeatures: vi.fn(),
}))

import { usePreferences } from '@/hooks/useSettings'
import { useFeatures } from '@/hooks/useFeatures'

describe('SummarySection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useFeatures).mockReturnValue({
      data: {
        summary_profiles: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFeatures>)
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
    render(<SummarySection />)
    expect(screen.getByText('Summary & Export')).toBeInTheDocument()
    expect(screen.getByText('Default summary and export settings')).toBeInTheDocument()
  })

  it('renders summary type radio group with all options', () => {
    render(<SummarySection />)
    expect(screen.getByText('General')).toBeInTheDocument()
    expect(screen.getByText('Key Points')).toBeInTheDocument()
    expect(screen.getByText('Action Items')).toBeInTheDocument()
    expect(screen.getByText('Questions & Answers')).toBeInTheDocument()
  })

  it('keeps professional profiles hidden until the feature is enabled', () => {
    render(<SummarySection />)

    expect(
      screen.queryByRole('combobox', { name: /professional profile/i })
    ).not.toBeInTheDocument()
  })

  it('renders all professional profiles when the feature is enabled', async () => {
    const user = userEvent.setup()
    vi.mocked(useFeatures).mockReturnValue({
      data: { summary_profiles: true },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFeatures>)
    render(<SummarySection />)

    const profileSelect = screen.getByRole('combobox', { name: /professional profile/i })
    expect(profileSelect).toHaveTextContent('General Professional')
    expect(screen.getByText(/does not give advice or verify claims/i)).toBeInTheDocument()

    await user.click(profileSelect)

    expect(screen.getByRole('option', { name: 'General Professional' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Legal' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Investment Analysis' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Journalism' })).toBeInTheDocument()
  })

  it('saves the selected professional profile', async () => {
    const user = userEvent.setup()
    vi.mocked(useFeatures).mockReturnValue({
      data: { summary_profiles: true },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFeatures>)
    render(<SummarySection />)

    await user.click(screen.getByRole('combobox', { name: /professional profile/i }))
    await user.click(screen.getByRole('option', { name: 'Legal' }))

    expect(mockMutate).toHaveBeenCalledWith({ summary_profile: 'legal' })
  })

  it('uses General Professional when the saved profile is absent or invalid', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['auto'],
        default_diarization: true,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.0,
        high_stakes_summaries: false,
        summary_profile: 'unknown_profile',
      } as unknown as UserPreferences,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    vi.mocked(useFeatures).mockReturnValue({
      data: { summary_profiles: true },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFeatures>)
    render(<SummarySection />)

    expect(screen.getByRole('combobox', { name: /professional profile/i })).toHaveTextContent(
      'General Professional'
    )
  })

  it('renders export format radio group with all options', () => {
    render(<SummarySection />)
    expect(screen.getByText('PDF')).toBeInTheDocument()
    expect(screen.getByText('DOCX')).toBeInTheDocument()
    expect(screen.getByText('JSON')).toBeInTheDocument()
  })

  it('current summary type is selected', () => {
    render(<SummarySection />)
    const generalRadio = screen.getByRole('radio', { name: /general/i })
    expect(generalRadio).toHaveAttribute('data-state', 'checked')
  })

  it('current export format is selected', () => {
    render(<SummarySection />)
    const pdfRadio = screen.getByRole('radio', { name: /pdf/i })
    expect(pdfRadio).toHaveAttribute('data-state', 'checked')
  })

  it('clicking a different summary type calls updatePreferences', async () => {
    const user = userEvent.setup()
    render(<SummarySection />)

    const keyPointsRadio = screen.getByRole('radio', { name: /key points/i })
    await user.click(keyPointsRadio)

    expect(mockMutate).toHaveBeenCalledWith({ default_summary_type: 'key_points' })
  })

  it('clicking Questions & Answers calls updatePreferences', async () => {
    const user = userEvent.setup()
    render(<SummarySection />)

    const questionsRadio = screen.getByRole('radio', { name: /questions & answers/i })
    await user.click(questionsRadio)

    expect(mockMutate).toHaveBeenCalledWith({ default_summary_type: 'q_and_a' })
  })

  it('clicking a different export format calls updatePreferences', async () => {
    const user = userEvent.setup()
    render(<SummarySection />)

    const docxRadio = screen.getByRole('radio', { name: /docx/i })
    await user.click(docxRadio)

    expect(mockMutate).toHaveBeenCalledWith({ default_export_format: 'docx' })
  })

  it('clicking high-stakes toggle calls updatePreferences', async () => {
    const user = userEvent.setup()
    render(<SummarySection />)

    await user.click(screen.getByRole('switch', { name: /high-stakes summaries/i }))

    expect(mockMutate).toHaveBeenCalledWith({ high_stakes_summaries: true })
  })

  it('always shows the high-stakes toggle (independent of deployment feature flags)', () => {
    render(<SummarySection />)

    expect(screen.getByRole('switch', { name: /high-stakes summaries/i })).toBeInTheDocument()
  })

  it('reflects the high-stakes preference state (default off)', () => {
    render(<SummarySection />)

    expect(screen.getByRole('switch', { name: /high-stakes summaries/i })).toHaveAttribute(
      'data-state',
      'unchecked'
    )
  })

  it('shows loading skeleton when preferences are loading', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<SummarySection />)
    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThan(0)
  })
})
