import { describe, it, expect, beforeEach } from 'vitest'
import { render, act, screen } from '@testing-library/react'
import { GuideLayout } from './GuideLayout'
import { useThemeStore } from '@/stores/theme'

describe('GuideLayout', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('forces light while mounted and restores a dark preference on unmount', () => {
    act(() => {
      useThemeStore.getState().setTheme('dark')
    })
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    const { unmount } = render(
      <GuideLayout>
        <div>guide</div>
      </GuideLayout>
    )
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    // App's own mount effect calling applyTheme() must not resurrect dark.
    act(() => {
      useThemeStore.getState().applyTheme()
    })
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    unmount()
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    // Leave the store light so this file has no side effects on later suites.
    act(() => {
      useThemeStore.getState().setTheme('light')
    })
  })

  // The landing page links here for anonymous visitors; the guide makes no API
  // calls, so it must render without any sign-in (no AuthProvider mounted).
  it('renders the guide without requiring sign-in', async () => {
    render(
      <GuideLayout>
        <div>guide body</div>
      </GuideLayout>
    )
    expect(await screen.findByText('guide body')).toBeInTheDocument()
  })
})
