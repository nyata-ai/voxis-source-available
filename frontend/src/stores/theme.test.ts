import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { act } from '@testing-library/react'

// We need to reset the store between tests
let useThemeStore: typeof import('./theme').useThemeStore
let setForceLightTheme: typeof import('./theme').setForceLightTheme

describe('useThemeStore', () => {
  beforeEach(async () => {
    localStorage.clear()
    document.documentElement.classList.remove('dark')
    // Re-import to get fresh store (also resets the module-level force counter)
    vi.resetModules()
    const mod = await import('./theme')
    useThemeStore = mod.useThemeStore
    setForceLightTheme = mod.setForceLightTheme
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('defaults to light theme', () => {
    const state = useThemeStore.getState()
    expect(state.theme).toBe('light')
  })

  it('setTheme("dark") adds dark class to documentElement', () => {
    act(() => {
      useThemeStore.getState().setTheme('dark')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('setTheme("light") removes dark class', () => {
    document.documentElement.classList.add('dark')
    act(() => {
      useThemeStore.getState().setTheme('light')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('setTheme("system") resolves based on prefers-color-scheme', () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    act(() => {
      useThemeStore.getState().setTheme('system')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('applyTheme reapplies current theme to document', () => {
    act(() => {
      useThemeStore.getState().setTheme('dark')
    })
    document.documentElement.classList.remove('dark')
    act(() => {
      useThemeStore.getState().applyTheme()
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('setTheme("dark") then "light" correctly toggles class', () => {
    act(() => {
      useThemeStore.getState().setTheme('dark')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    act(() => {
      useThemeStore.getState().setTheme('light')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('system theme with dark OS preference adds dark class', () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    act(() => {
      useThemeStore.getState().setTheme('system')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('system theme with light OS preference does not add dark class', () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    act(() => {
      useThemeStore.getState().setTheme('system')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  describe('setForceLightTheme', () => {
    it('forcing wins over a stored dark preference, even across applyTheme()', () => {
      act(() => {
        useThemeStore.getState().setTheme('dark')
        setForceLightTheme(true)
      })
      expect(document.documentElement.classList.contains('dark')).toBe(false)

      // A later applyTheme (e.g. App's mount effect) must not resurrect dark.
      act(() => {
        useThemeStore.getState().applyTheme()
      })
      expect(document.documentElement.classList.contains('dark')).toBe(false)
    })

    it('nested owners keep forcing until the last one releases', () => {
      act(() => {
        useThemeStore.getState().setTheme('dark')
        setForceLightTheme(true)
        setForceLightTheme(true)
        setForceLightTheme(false)
      })
      expect(document.documentElement.classList.contains('dark')).toBe(false)

      act(() => {
        setForceLightTheme(false)
      })
      expect(document.documentElement.classList.contains('dark')).toBe(true)
    })

    it('a balanced force/release restores the dark preference and clamps over-release', () => {
      act(() => {
        useThemeStore.getState().setTheme('dark')
        setForceLightTheme(true)
        setForceLightTheme(false)
      })
      expect(document.documentElement.classList.contains('dark')).toBe(true)

      // An extra release must not corrupt the counter for the next owner.
      act(() => {
        setForceLightTheme(false)
        setForceLightTheme(true)
      })
      expect(document.documentElement.classList.contains('dark')).toBe(false)
    })
  })
})
