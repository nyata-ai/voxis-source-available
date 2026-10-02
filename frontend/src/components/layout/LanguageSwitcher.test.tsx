import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from '@/lib/toast'
import { LanguageSwitcher } from './LanguageSwitcher'

const mockMutate = vi.fn()
const mockChangeAppLanguage = vi.fn()

vi.mock('@/hooks/useSettings', () => ({
  useUpdatePreferences: vi.fn(() => ({ mutate: mockMutate })),
}))

vi.mock('@/lib/toast', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

// The active language is read from the real i18n instance (default `en` in tests);
// only the imperative switch is mocked so it never mutates the shared singleton.
vi.mock('@/i18n/language', () => ({
  changeAppLanguage: (code: string) => mockChangeAppLanguage(code),
}))

describe('LanguageSwitcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows the active language short code on the trigger', () => {
    render(<LanguageSwitcher />)
    const trigger = screen.getByRole('button', { name: /language/i })
    expect(within(trigger).getByText('EN')).toBeInTheDocument()
  })

  it('lists translated languages with native labels when opened', async () => {
    const user = userEvent.setup()
    render(<LanguageSwitcher />)
    await user.click(screen.getByRole('button', { name: /language/i }))

    const menu = screen.getByRole('menu')
    expect(within(menu).getByText('Bahasa Indonesia')).toBeInTheDocument()
    expect(within(menu).getByText('Deutsch')).toBeInTheDocument()
    expect(within(menu).getByText('简体中文')).toBeInTheDocument()
    // Non-picker locales are no longer offered (only the active one would appear).
    expect(within(menu).queryByText('日本語')).not.toBeInTheDocument()
    expect(within(menu).queryByText('Русский')).not.toBeInTheDocument()
  })

  it('switches and persists the language on selection', async () => {
    const user = userEvent.setup()
    render(<LanguageSwitcher />)
    await user.click(screen.getByRole('button', { name: /language/i }))
    await user.click(screen.getByRole('menuitemradio', { name: /Bahasa Indonesia/i }))

    expect(mockChangeAppLanguage).toHaveBeenCalledWith('id')
    expect(mockMutate).toHaveBeenCalledWith(
      { ui_language: 'id' },
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
    render(<LanguageSwitcher />)
    await user.click(screen.getByRole('button', { name: /language/i }))
    await user.click(screen.getByRole('menuitemradio', { name: /Bahasa Indonesia/i }))

    expect(toast.error).toHaveBeenCalledTimes(1)
  })

  it('does nothing when the active language is re-selected', async () => {
    const user = userEvent.setup()
    render(<LanguageSwitcher />)
    await user.click(screen.getByRole('button', { name: /language/i }))
    await user.click(screen.getByRole('menuitemradio', { name: /English/i }))

    expect(mockChangeAppLanguage).not.toHaveBeenCalled()
    expect(mockMutate).not.toHaveBeenCalled()
  })
})
