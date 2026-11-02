import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'

function LocationProbe() {
  const location = useLocation()
  return <output data-testid="location">{location.search}</output>
}

function renderSettings(initialEntry = '/settings') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SettingsPage />
      <LocationProbe />
    </MemoryRouter>
  )
}

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
    renderSettings()
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
    renderSettings()
    expect(screen.queryByText('Notifications')).not.toBeInTheDocument()
  })

  it('default active section is Profile', () => {
    renderSettings()
    expect(screen.getByTestId('profile-section')).toBeInTheDocument()
  })

  it('clicking Appearance shows AppearanceSection', async () => {
    const user = userEvent.setup()
    renderSettings()
    await user.click(screen.getByText('Appearance'))
    expect(screen.getByTestId('appearance-section')).toBeInTheDocument()
  })

  it('clicking each nav item switches content area', async () => {
    const user = userEvent.setup()
    renderSettings()

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
    renderSettings()
    expect(screen.getByRole('heading', { name: /settings/i })).toBeInTheDocument()
  })

  describe('section in the URL', () => {
    it('opens the section named by ?section=', () => {
      renderSettings('/settings?section=api-keys')
      expect(screen.getByTestId('apikeys-section')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'API Keys' })).toHaveAttribute(
        'aria-current',
        'page'
      )
    })

    it('falls back to Profile for an unknown section', () => {
      renderSettings('/settings?section=nope')
      expect(screen.getByTestId('profile-section')).toBeInTheDocument()
    })

    it('writes the chosen section to the URL, and drops it for Profile', async () => {
      const user = userEvent.setup()
      renderSettings()

      await user.click(screen.getByRole('button', { name: 'Language' }))
      expect(screen.getByTestId('location')).toHaveTextContent('?section=language')

      await user.click(screen.getByRole('button', { name: 'Profile' }))
      expect(screen.getByTestId('location')).toBeEmptyDOMElement()
    })
  })

  it('leaves Settings on Back, however many sections were opened', async () => {
    const user = userEvent.setup()
    function BackButton() {
      const navigate = useNavigate()
      return <button onClick={() => navigate(-1)}>Back</button>
    }
    render(
      <MemoryRouter initialEntries={['/desk', '/settings']} initialIndex={1}>
        <Routes>
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/desk" element={<p>The Desk</p>} />
        </Routes>
        <BackButton />
      </MemoryRouter>
    )

    await user.click(screen.getByRole('button', { name: 'Language' }))
    await user.click(screen.getByRole('button', { name: 'Playback' }))
    await user.click(screen.getByRole('button', { name: 'Back' }))

    expect(screen.getByText('The Desk')).toBeInTheDocument()
  })

  it('marks the active item with the coral edge, like the sidebar', () => {
    renderSettings('/settings?section=playback')
    const active = screen.getByRole('button', { name: 'Playback' })
    expect(active).toHaveClass('border-coral-mark')
    expect(screen.getByRole('button', { name: 'Profile' })).not.toHaveAttribute('aria-current')
  })
})
