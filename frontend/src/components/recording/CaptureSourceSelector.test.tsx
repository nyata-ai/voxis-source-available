import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { CaptureSourceSelector } from './CaptureSourceSelector'

describe('CaptureSourceSelector', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('selects microphone capture by default', () => {
    render(
      <CaptureSourceSelector
        value="microphone"
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('radio', { name: /microphone/i })).toBeChecked()
    expect(screen.getByRole('radio', { name: /meeting audio/i })).not.toBeChecked()
  })

  it('calls onChange when meeting audio is selected', () => {
    const onChange = vi.fn()
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia: vi.fn() },
      configurable: true,
    })

    render(
      <CaptureSourceSelector
        value="microphone"
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('radio', { name: /meeting audio/i }))

    expect(onChange).toHaveBeenCalledWith('mixed_audio')
  })

  it('disables meeting audio when display capture is unavailable', () => {
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {},
      configurable: true,
    })

    render(
      <CaptureSourceSelector
        value="microphone"
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('radio', { name: /meeting audio/i })).toBeDisabled()
    expect(
      screen.getByText('This browser cannot open a sharing picker. Use Chrome or Edge to share tab audio.'),
    ).toBeInTheDocument()
  })

  it('keeps meeting audio available from the display-capture API without browser sniffing', () => {
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getDisplayMedia: vi.fn() },
      configurable: true,
    })

    render(<CaptureSourceSelector value="mixed_audio" onChange={vi.fn()} />)

    expect(screen.getByRole('radio', { name: /meeting audio/i })).toBeEnabled()
    expect(
      screen.getByText('Chrome and Edge can share tab audio. Full-screen, native-app, and system audio depend on your browser and operating system.'),
    ).toBeInTheDocument()
  })
})
