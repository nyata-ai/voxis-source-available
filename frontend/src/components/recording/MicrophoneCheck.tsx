import { useTranslation } from 'react-i18next'
import { Mic } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { RecordingWaveform } from './RecordingWaveform'
import { useMicrophoneCheck } from './useMicrophoneCheck'

interface MicrophoneCheckProps {
  /** The microphone the recording will use; empty means the browser default. */
  deviceId: string
  /** The page is starting its own capture. Some browsers end one capture when
   *  another opens, so the check lets go of the microphone and stays out. */
  disabled?: boolean
}

/**
 * An opt-in "can Voxis hear me?" check before recording. This check opens the
 * microphone only when the user presses its button, and releases it after ten
 * seconds, on unmount, when the chosen microphone changes, when `disabled` turns
 * true, or when the page moves on to recording. (The page itself may already have
 * asked for the microphone once to read device names: see `useMicrophoneDevices`.)
 */
export function MicrophoneCheck({ deviceId, disabled = false }: MicrophoneCheckProps) {
  const { t } = useTranslation('recording')
  const { state, heard, silent, failed, analyser, start, onLevel } = useMicrophoneCheck(deviceId, disabled)

  const listening = state === 'listening'
  const busy = state === 'starting' || listening
  const message = failed
    ? t('recorder.micDenied')
    : heard
      ? t('micCheck.heard')
      : (listening && silent) || state === 'done'
        ? t('micCheck.silent')
        : ''

  return (
    <div className="flex w-full flex-col items-start gap-3">
      {/* `aria-disabled` while busy, not `disabled`: a button that disables
          itself drops keyboard focus to the page. */}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => {
          if (!busy) void start()
        }}
        disabled={disabled}
        aria-disabled={busy || undefined}
        className="gap-2 aria-disabled:cursor-not-allowed aria-disabled:opacity-50 max-sm:h-11"
      >
        <Mic className="h-4 w-4" aria-hidden="true" />
        {busy ? t('micCheck.listening') : t('micCheck.test')}
      </Button>
      {listening && (
        <RecordingWaveform analyserNode={analyser} isPaused={false} tone="test" onLevel={onLevel} />
      )}
      <p role="status" aria-live="polite" className="min-h-5 text-sm text-muted-foreground">
        {message}
      </p>
    </div>
  )
}
