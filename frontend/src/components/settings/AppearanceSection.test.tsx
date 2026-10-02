import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AppearanceSection } from './AppearanceSection'

const mockSetTheme = vi.fn()
const mockMutate = vi.fn()

vi.mock('@/stores/theme', () => ({
  useThemeStore: vi.fn(() => ({
    theme: 'system' as const,
    setTheme: mockSetTheme,
  })),
}))

vi.mock('@/hooks/useSettings', () => ({
  useUpdatePreferences: vi.fn(() => ({
    mutate: mockMutate,
  })),
}))

import { useThemeStore } from '@/stores/theme'

/** Helper to find a theme button by its label text */
function getThemeButton(name: string): HTMLElement {
  const label = screen.getByText(name)
  return label.closest('button')!
}

describe('AppearanceSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useThemeStore).mockReturnValue({
      theme: 'system',
      setTheme: mockSetTheme,
      applyTheme: vi.fn(),
    })
  })

  it('renders section heading', () => {
    render(<AppearanceSection />)
    expect(screen.getByText('Appearance')).toBeInTheDocument()
    expect(screen.getByText('Customize the look and feel')).toBeInTheDocument()
  })

  it('renders three theme options', () => {
    render(<AppearanceSection />)
    expect(screen.getByText('Light')).toBeInTheDocument()
    expect(screen.getByText('Dark')).toBeInTheDocument()
    expect(screen.getByText('System')).toBeInTheDocument()
  })

  it('highlights the current theme (system by default)', () => {
    render(<AppearanceSection />)
    const systemButton = getThemeButton('System')
    expect(systemButton.className).toMatch(/ring-2/)
  })

  it('clicking Light calls setTheme and updatePreferences', async () => {
    const user = userEvent.setup()
    render(<AppearanceSection />)

    await user.click(getThemeButton('Light'))

    expect(mockSetTheme).toHaveBeenCalledWith('light')
    expect(mockMutate).toHaveBeenCalledWith({ theme: 'light' })
  })

  it('clicking Dark calls setTheme and updatePreferences', async () => {
    const user = userEvent.setup()
    render(<AppearanceSection />)

    await user.click(getThemeButton('Dark'))

    expect(mockSetTheme).toHaveBeenCalledWith('dark')
    expect(mockMutate).toHaveBeenCalledWith({ theme: 'dark' })
  })

  it('clicking System calls setTheme and updatePreferences', async () => {
    const user = userEvent.setup()
    vi.mocked(useThemeStore).mockReturnValue({
      theme: 'dark',
      setTheme: mockSetTheme,
      applyTheme: vi.fn(),
    })
    render(<AppearanceSection />)

    await user.click(getThemeButton('System'))

    expect(mockSetTheme).toHaveBeenCalledWith('system')
    expect(mockMutate).toHaveBeenCalledWith({ theme: 'system' })
  })

  it('visually selects dark theme when active', () => {
    vi.mocked(useThemeStore).mockReturnValue({
      theme: 'dark',
      setTheme: mockSetTheme,
      applyTheme: vi.fn(),
    })
    render(<AppearanceSection />)

    const darkButton = getThemeButton('Dark')
    expect(darkButton.className).toMatch(/ring-2/)
  })
})
