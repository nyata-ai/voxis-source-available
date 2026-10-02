import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlaybackSection } from './PlaybackSection'

const mockMutate = vi.fn()

vi.mock('@/hooks/useSettings', () => ({
  usePreferences: vi.fn(),
  useUpdatePreferences: vi.fn(() => ({
    mutate: mockMutate,
  })),
}))

import { usePreferences } from '@/hooks/useSettings'

describe('PlaybackSection', () => {
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
    render(<PlaybackSection />)
    expect(screen.getByText('Playback')).toBeInTheDocument()
    expect(screen.getByText('Audio playback preferences')).toBeInTheDocument()
  })

  it('renders speed label', () => {
    render(<PlaybackSection />)
    expect(screen.getByText('Playback Speed')).toBeInTheDocument()
  })

  it('displays current speed value', () => {
    render(<PlaybackSection />)
    // The speed display is in a font-medium span alongside the label
    const speedDisplays = screen.getAllByText('1.0x')
    expect(speedDisplays.length).toBeGreaterThanOrEqual(1)
  })

  it('renders slider element', () => {
    render(<PlaybackSection />)
    const slider = screen.getByRole('slider')
    expect(slider).toBeInTheDocument()
  })

  it('slider has correct min/max/step attributes', () => {
    render(<PlaybackSection />)
    const slider = screen.getByRole('slider')
    expect(slider).toHaveAttribute('aria-valuemin', '0.5')
    expect(slider).toHaveAttribute('aria-valuemax', '2')
    expect(slider).toHaveAttribute('aria-valuenow', '1')
  })

  it('displays different speed when preferences differ', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: {
        theme: 'system',
        default_languages: ['auto'],
        default_diarization: true,
        default_summary_type: 'general',
        default_export_format: 'pdf',
        playback_speed: 1.25,
        high_stakes_summaries: false,
      },
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<PlaybackSection />)
    // 1.25 is displayed - only appears in speed display, not in markers
    const slider = screen.getByRole('slider')
    expect(slider).toHaveAttribute('aria-valuenow', '1.25')
  })

  it('shows loading skeleton when preferences are loading', () => {
    vi.mocked(usePreferences).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    } as ReturnType<typeof usePreferences>)

    render(<PlaybackSection />)
    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThan(0)
  })
})
