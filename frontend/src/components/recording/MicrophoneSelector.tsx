import { Mic } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { MicrophoneDevice } from '@/hooks/useMicrophoneDevices'

interface MicrophoneSelectorProps {
  devices: MicrophoneDevice[]
  selectedDeviceId: string
  onDeviceChange: (deviceId: string) => void
  disabled?: boolean
}

/**
 * Dropdown to pick a microphone device.
 * Shows a simple label if only one device is available.
 */
export function MicrophoneSelector({
  devices,
  selectedDeviceId,
  onDeviceChange,
  disabled = false,
}: MicrophoneSelectorProps) {
  const { t } = useTranslation('recording')
  if (devices.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Mic className="h-4 w-4" />
        <span>{t('mic.none')}</span>
      </div>
    )
  }

  if (devices.length === 1) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Mic className="h-4 w-4" />
        <span>{devices[0].label}</span>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <Mic className="h-4 w-4 text-muted-foreground shrink-0" />
      <Select value={selectedDeviceId} onValueChange={onDeviceChange} disabled={disabled}>
        <SelectTrigger className="w-64" aria-label={t('mic.select')}>
          <SelectValue placeholder={t('mic.select')} />
        </SelectTrigger>
        <SelectContent>
          {devices.map((device) => (
            <SelectItem key={device.deviceId} value={device.deviceId}>
              {device.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
