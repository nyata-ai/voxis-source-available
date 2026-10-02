import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SettingsPage } from './SettingsPage'

// Mock child sections to isolate layout testing
vi.mock('@/components/settings/ProfileSection', () => ({
  ProfileSection: () => <div data-testid="profile-section">Profile Content</div>,
}))
vi.mock('@/components/settings/AppearanceSection', () => ({
  AppearanceSection: () => <div data-testid="appearance-section">Appearance Content</div>,
}))
vi.mock('@/components/settings/LanguageSection', () => ({
  LanguageSection: () => <div data-testid="language-section">Language Content</div>,
}))
vi.mock('@/components/settings/TranscriptionSection', () => ({
  TranscriptionSection: () => <div data-testid="transcription-section">Transcription Content</div>,
}))
vi.mock('@/components/settings/SummarySection', () => ({
  SummarySection: () => <div data-testid="summary-section">Summary Content</div>,
}))
vi.mock('@/components/settings/PlaybackSection', () => ({
  PlaybackSection: () => <div data-testid="playback-section">Playback Content</div>,
}))
vi.mock('@/components/settings/ApiKeysSection', () => ({
  ApiKeysSection: () => <div data-testid="apikeys-section">API Keys Content</div>,
}))
describe('SettingsPage', () => {
  it('renders with sidebar navigation showing internal-profile section names', () => {
    render(<SettingsPage />)
    expect(screen.getByText('Profile')).toBeInTheDocument()
    expect(screen.getByText('Appearance')).toBeInTheDocument()
    expect(screen.getByText('Language')).toBeInTheDocument()
    expect(screen.getByText('Transcription')).toBeInTheDocument()
    expect(screen.getByText('Summary & Export')).toBeInTheDocument()
    expect(screen.getByText('Playback')).toBeInTheDocument()
    expect(screen.queryByText('Recording Retention')).not.toBeInTheDocument()
    expect(screen.queryByText('Notifications')).not.toBeInTheDocument()
    expect(screen.getByText('API Keys')).toBeInTheDocument()
  })

  it('does not expose email notification controls', () => {
    render(<SettingsPage />)
    expect(screen.queryByText('Notifications')).not.toBeInTheDocument()
  })

  it('default active section is Profile', () => {
    render(<SettingsPage />)
    expect(screen.getByTestId('profile-section')).toBeInTheDocument()
  })

  it('clicking Appearance shows AppearanceSection', async () => {
    const user = userEvent.setup()
    render(<SettingsPage />)
    await user.click(screen.getByText('Appearance'))
    expect(screen.getByTestId('appearance-section')).toBeInTheDocument()
  })

  it('clicking each nav item switches content area', async () => {
    const user = userEvent.setup()
    render(<SettingsPage />)

    await user.click(screen.getByText('Language'))
    expect(screen.getByTestId('language-section')).toBeInTheDocument()

    await user.click(screen.getByText('Transcription'))
    expect(screen.getByTestId('transcription-section')).toBeInTheDocument()

    await user.click(screen.getByText('Summary & Export'))
    expect(screen.getByTestId('summary-section')).toBeInTheDocument()

    await user.click(screen.getByText('Playback'))
    expect(screen.getByTestId('playback-section')).toBeInTheDocument()

    await user.click(screen.getByText('API Keys'))
    expect(screen.getByTestId('apikeys-section')).toBeInTheDocument()
  })

  it('renders page title', () => {
    render(<SettingsPage />)
    expect(screen.getByRole('heading', { name: /settings/i })).toBeInTheDocument()
  })
})
