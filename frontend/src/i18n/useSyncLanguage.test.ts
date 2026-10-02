import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSyncLanguage } from './useSyncLanguage'
import { LANGUAGE_STORAGE_KEY } from './config'

const mockUsePreferences = vi.fn()
const mockChangeAppLanguage = vi.fn()
const mockApplyServerLanguage = vi.fn()
const mockCurrentLanguage = vi.fn(() => 'en')
const mockRequestedLanguage = vi.fn(() => 'en')

vi.mock('@/hooks/useSettings', () => ({
  usePreferences: () => mockUsePreferences(),
}))

vi.mock('./language', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./language')>()
  return {
    changeAppLanguage: (code: string) => mockChangeAppLanguage(code),
    currentLanguage: () => mockCurrentLanguage(),
    requestedLanguage: () => mockRequestedLanguage(),
    // Spied but NOT stubbed: the real implementation still runs, so the
    // "leaves localStorage alone" assertion below tests the shipped code
    // rather than the mock.
    applyServerLanguage: (code: string) => {
      mockApplyServerLanguage(code)
      return actual.applyServerLanguage(code)
    },
  }
})

describe('useSyncLanguage', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
    vi.clearAllMocks()
    mockCurrentLanguage.mockReturnValue('en')
    mockRequestedLanguage.mockReturnValue('en')
  })

  it('does not override an explicit public language URL with a saved preference', () => {
    window.history.replaceState({}, '', '/id')
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'de' } })
    renderHook(() => useSyncLanguage())
    expect(mockApplyServerLanguage).not.toHaveBeenCalled()
    window.history.replaceState({}, '', '/')
  })

  it('restores the saved language when leaving an explicit landing URL', () => {
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'de' } })
    const { rerender } = renderHook(({ path }) => useSyncLanguage(path), { initialProps: { path: '/id' } })
    expect(mockApplyServerLanguage).not.toHaveBeenCalled()
    rerender({ path: '/dashboard' })
    expect(mockApplyServerLanguage).toHaveBeenCalledWith('de')
  })

  it('changes language when preference differs from current', () => {
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'de' } })
    renderHook(() => useSyncLanguage())
    expect(mockApplyServerLanguage).toHaveBeenCalledWith('de')
  })

  it('does nothing when preference equals current language', () => {
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'en' } })
    renderHook(() => useSyncLanguage())
    expect(mockApplyServerLanguage).not.toHaveBeenCalled()
  })

  // i18next only advances resolvedLanguage once a catalog for the target has
  // loaded, so every non-English boot has a window where the requested language
  // is already 'id' but the one on screen is still 'en'. A genuine English
  // choice made on another device that arrives inside that window used to
  // compare 'en' !== 'en', no-op, and never retry (the effect depends only on
  // the preference), leaving the user stuck in Indonesian for the session.
  it('applies a server preference that matches the still-displayed language', () => {
    mockCurrentLanguage.mockReturnValue('en')
    mockRequestedLanguage.mockReturnValue('id')
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'en' } })
    renderHook(() => useSyncLanguage())
    expect(mockApplyServerLanguage).toHaveBeenCalledWith('en')
  })

  it('does nothing when no preference is set', () => {
    mockUsePreferences.mockReturnValue({ data: undefined })
    renderHook(() => useSyncLanguage())
    expect(mockApplyServerLanguage).not.toHaveBeenCalled()
  })

  it('leaves localStorage untouched when applying a server preference', () => {
    localStorage.clear()
    mockUsePreferences.mockReturnValue({ data: { ui_language: 'de' } })
    renderHook(() => useSyncLanguage())
    // An older backend build sends "en" for every user. Persisting that here
    // would outrank detection on this device and survive the backend upgrade.
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBeNull()
    expect(mockChangeAppLanguage).not.toHaveBeenCalled()
  })
})
