import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from '@/lib/toast'
import { LanguageSection } from './LanguageSection'

const mockMutate = vi.fn()
const mockChangeAppLanguage = vi.fn()
const mockCurrentLanguage = vi.fn(() => 'en')

vi.mock('@/hooks/useSettings', () => ({
  useUpdatePreferences: vi.fn(() => ({
    mutate: mockMutate,
  })),
}))

vi.mock('@/lib/toast', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

vi.mock('../../i18n/language', () => ({
  changeAppLanguage: (code: string) => mockChangeAppLanguage(code),
  currentLanguage: () => mockCurrentLanguage(),
}))

/** Helper to find a locale button by any of its text labels */
function getLocaleButton(name: string): HTMLElement {
  const label = screen.getAllByText(name)[0]
  return label.closest('button')!
}

describe('LanguageSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockCurrentLanguage.mockReturnValue('en')
  })

  it('renders section heading', () => {
    render(<LanguageSection />)
    expect(screen.getByText('Language')).toBeInTheDocument()
    expect(screen.getByText('Choose the language for the Voxis interface')).toBeInTheDocument()
  })

  it('renders native labels for locales', () => {
    render(<LanguageSection />)
    // 'English' appears as both native + English label for the en locale.
    expect(screen.getAllByText('English').length).toBeGreaterThan(0)
    expect(screen.getByText('Deutsch')).toBeInTheDocument()
    expect(screen.getByText('Bahasa Indonesia')).toBeInTheDocument()
    expect(screen.getByText('简体中文')).toBeInTheDocument()
    // Non-picker locales only render while active — not for an English reader.
    expect(screen.queryByText('日本語')).not.toBeInTheDocument()
  })

  it('highlights the current language (English by default)', () => {
    render(<LanguageSection />)
    // The active locale is the only button with aria-pressed=true.
    const active = screen.getByRole('button', { pressed: true })
    expect(active).toHaveTextContent('English')
    expect(active.className).toMatch(/ring-2/)
    // Non-active locales are neither pressed nor highlighted.
    const deutsch = getLocaleButton('Deutsch')
    expect(deutsch).toHaveAttribute('aria-pressed', 'false')
    expect(deutsch.className).not.toMatch(/ring-2/)
  })

  it('allows selecting shipped translated locales', async () => {
    const user = userEvent.setup()
    render(<LanguageSection />)

    const german = getLocaleButton('Deutsch')
    expect(german).not.toBeDisabled()

    await user.click(german)

    expect(mockChangeAppLanguage).toHaveBeenCalledWith('de')
    expect(mockMutate).toHaveBeenCalledWith(
      { ui_language: 'de' },
      { onError: expect.any(Function) },
    )
  })

  it('keeps English selectable', async () => {
    const user = userEvent.setup()
    render(<LanguageSection />)

    const english = getLocaleButton('English')
    expect(english).not.toBeDisabled()

    await user.click(english)

    expect(mockChangeAppLanguage).toHaveBeenCalledWith('en')
    expect(mockMutate).toHaveBeenCalledWith(
      { ui_language: 'en' },
      { onError: expect.any(Function) },
    )
  })

  // Persisting is one-way (the backend rejects an empty ui_language), so this
  // click is the only way to pin a detected language. Swallowing it would leave
  // a reader detected as Indonesian with no way to make that stick.
  it('persists a re-selection of the already active language', async () => {
    mockCurrentLanguage.mockReturnValue('de')
    const user = userEvent.setup()
    render(<LanguageSection />)

    await user.click(getLocaleButton('Deutsch'))

    expect(mockChangeAppLanguage).toHaveBeenCalledWith('de')
    expect(mockMutate).toHaveBeenCalledWith(
      { ui_language: 'de' },
      { onError: expect.any(Function) },
    )
  })

  // A silent failure is the worst outcome available: the switch applies locally,
  // then the next refetch hands the stale server value back to useSyncLanguage
  // and the UI flips back with no explanation.
  it('reports a failed preference write', async () => {
    mockMutate.mockImplementation((_prefs: unknown, opts: { onError: () => void }) => {
      opts.onError()
    })
    const user = userEvent.setup()
    render(<LanguageSection />)

    await user.click(getLocaleButton('Deutsch'))

    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('visually selects German when active', () => {
    mockCurrentLanguage.mockReturnValue('de')
    render(<LanguageSection />)

    const germanButton = getLocaleButton('Deutsch')
    expect(germanButton).not.toBeDisabled()
    expect(germanButton).toHaveAttribute('aria-pressed', 'true')
    expect(germanButton.className).toMatch(/ring-2/)
  })
})
