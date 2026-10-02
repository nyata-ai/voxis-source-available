import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MicrophoneSelector } from './MicrophoneSelector'

describe('MicrophoneSelector', () => {
  it('shows "No microphone detected" when devices is empty', () => {
    render(
      <MicrophoneSelector
        devices={[]}
        selectedDeviceId=""
        onDeviceChange={vi.fn()}
      />,
    )

    expect(screen.getByText('No microphone detected')).toBeInTheDocument()
  })

  it('shows device label directly when only one device', () => {
    render(
      <MicrophoneSelector
        devices={[{ deviceId: 'dev-1', label: 'Built-in Microphone' }]}
        selectedDeviceId="dev-1"
        onDeviceChange={vi.fn()}
      />,
    )

    expect(screen.getByText('Built-in Microphone')).toBeInTheDocument()
    // Should not render a dropdown
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  })

  it('renders a dropdown when multiple devices available', () => {
    render(
      <MicrophoneSelector
        devices={[
          { deviceId: 'dev-1', label: 'Built-in Microphone' },
          { deviceId: 'dev-2', label: 'External USB Mic' },
        ]}
        selectedDeviceId="dev-1"
        onDeviceChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('combobox', { name: 'Select microphone' })).toBeInTheDocument()
  })

  it('dropdown is disabled when disabled prop is true', () => {
    render(
      <MicrophoneSelector
        devices={[
          { deviceId: 'dev-1', label: 'Mic 1' },
          { deviceId: 'dev-2', label: 'Mic 2' },
        ]}
        selectedDeviceId="dev-1"
        onDeviceChange={vi.fn()}
        disabled
      />,
    )

    expect(screen.getByRole('combobox')).toBeDisabled()
  })
})
