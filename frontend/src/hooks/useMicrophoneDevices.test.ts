import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useMicrophoneDevices } from './useMicrophoneDevices'

describe('useMicrophoneDevices', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads audio input devices and auto-selects first device', async () => {
    const stop = vi.fn()
    const getUserMedia = vi.fn().mockResolvedValue({
      getTracks: () => [{ stop }],
    })
    const enumerateDevices = vi.fn().mockResolvedValue([
      { kind: 'audioinput', deviceId: 'mic-1', label: 'Built-in Mic' },
      { kind: 'audioinput', deviceId: 'mic-2', label: 'USB Mic' },
    ])

    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia, enumerateDevices },
      configurable: true,
    })

    const { result } = renderHook(() => useMicrophoneDevices())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    expect(result.current.error).toBeNull()
    expect(result.current.devices).toHaveLength(2)
    expect(result.current.selectedDeviceId).toBe('mic-1')
    expect(stop).toHaveBeenCalled()
  })

  it('sets a clear error on permission denial', async () => {
    const getUserMedia = vi
      .fn()
      .mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'))
    const enumerateDevices = vi.fn()

    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia, enumerateDevices },
      configurable: true,
    })

    const { result } = renderHook(() => useMicrophoneDevices())

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false)
    })

    expect(result.current.error).toContain('Microphone access denied')
    expect(result.current.devices).toHaveLength(0)
  })
})
