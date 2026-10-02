import { useTranslation } from 'react-i18next'

interface RecordingTimerProps {
  /** Elapsed time in seconds. */
  elapsed: number
  /** Whether the timer is paused. */
  isPaused: boolean
}

/**
 * Formats elapsed seconds as HH:MM:SS and displays it.
 * Shows a visual cue when paused.
 */
export function RecordingTimer({ elapsed, isPaused }: RecordingTimerProps) {
  const { t } = useTranslation('recording')
  const hours = Math.floor(elapsed / 3600)
  const minutes = Math.floor((elapsed % 3600) / 60)
  const seconds = Math.floor(elapsed % 60)

  const pad = (n: number) => n.toString().padStart(2, '0')
  const display = `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`

  return (
    <div className="flex items-center gap-3" role="timer" aria-label={t('timer.ariaLabel', { display })}>
      <span
        className={`font-mono text-5xl font-bold tracking-wider tabular-nums ${
          isPaused ? 'text-muted-foreground' : 'text-foreground'
        }`}
      >
        {display}
      </span>
    </div>
  )
}
