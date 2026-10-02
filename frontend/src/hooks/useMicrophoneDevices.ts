import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

export interface MicrophoneDevice {
  deviceId: string
  label: string
}

/**
 * Enumerates audio input devices. Requests mic permission if needed
 * to get labeled device info (browsers hide labels until permission granted).
 *
 * Returns:
 * - devices: list of audio input devices
 * - selectedDeviceId: currently selected device
 * - setSelectedDeviceId: setter for selection
 * - error: permission or enumeration error message
 * - isLoading: true while enumerating
 */
export function useMicrophoneDevices() {
  const { t } = useTranslation('recording')
  const [devices, setDevices] = useState<MicrophoneDevice[]>([])
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>('')
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    let cancelled = false

    async function enumerate() {
      try {
        // Request mic permission first (labels are hidden until granted)
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
        // Release the stream immediately
        for (const track of stream.getTracks()) {
          track.stop()
        }

        const allDevices = await navigator.mediaDevices.enumerateDevices()
        if (cancelled) return

        const audioInputs = allDevices
          .filter((d) => d.kind === 'audioinput')
          .map((d) => ({
            deviceId: d.deviceId,
            label: d.label || t('micDevices.fallbackLabel', { id: d.deviceId.slice(0, 8) }),
          }))

        setDevices(audioInputs)
        // Auto-select first device if none selected
        if (audioInputs.length > 0 && !selectedDeviceId) {
          setSelectedDeviceId(audioInputs[0].deviceId)
        }
      } catch (err) {
        if (cancelled) return
        if (err instanceof DOMException) {
          if (err.name === 'NotAllowedError') {
            setError(t('micDevices.denied'))
          } else if (err.name === 'NotFoundError') {
            setError(t('micDevices.notFound'))
          } else {
            setError(t('micDevices.error', { message: err.message }))
          }
        } else {
          setError(t('micDevices.failed'))
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false)
        }
      }
    }

    enumerate()
    return () => {
      cancelled = true
    }
  }, [t]) // eslint-disable-line react-hooks/exhaustive-deps -- run on mount and relabel on language change

  return { devices, selectedDeviceId, setSelectedDeviceId, error, isLoading }
}
