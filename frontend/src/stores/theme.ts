import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Theme = 'light' | 'dark' | 'system'

interface ThemeState {
  theme: Theme
  setTheme: (theme: Theme) => void
  applyTheme: () => void
}

// While > 0, the document renders light regardless of the stored preference.
// Set only by full-page routes designed against the light system (the guide).
// A counter — not a boolean, and not store state (it must never persist) — so
// that if a second force-light route ever mounts while the first is still up,
// neither unmount can turn forcing off underneath the other.
let forceLightCount = 0

function applyToDocument(theme: Theme) {
  const root = document.documentElement
  const isDark =
    forceLightCount === 0 &&
    (theme === 'dark' ||
      (theme === 'system' &&
        typeof window.matchMedia === 'function' &&
        window.matchMedia('(prefers-color-scheme: dark)').matches))
  root.classList.toggle('dark', isDark)
}

/**
 * Force the light theme on/off for the lifetime of a mounted route. Re-applies
 * immediately, and every later applyTheme()/setTheme() call respects the count,
 * so a parent effect running after the caller cannot resurrect dark mode.
 * Callers must balance each `true` with exactly one `false` (mount/unmount).
 */
export function setForceLightTheme(on: boolean) {
  forceLightCount = Math.max(0, forceLightCount + (on ? 1 : -1))
  applyToDocument(useThemeStore.getState().theme)
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      // Default is 'light' (this app's editorial system). The backend's
      // DefaultPreferences still reports 'system' and the server-side theme
      // preference is write-only today (nothing hydrates the store from it),
      // so anyone wiring server→store hydration must not silently resurrect
      // 'system' as the effective default here.
      theme: 'light' as Theme,
      setTheme: (theme: Theme) => {
        set({ theme })
        applyToDocument(theme)
      },
      applyTheme: () => applyToDocument(get().theme),
    }),
    { name: 'voxis-theme' }
  )
)
