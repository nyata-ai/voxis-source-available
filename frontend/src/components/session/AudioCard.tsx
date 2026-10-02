import { forwardRef, type Ref } from 'react'
import { AudioPlayer, type AudioPlayerHandle } from '@/components/media/AudioPlayer'
import type { SessionAudio } from './session-view'

export interface AudioCardProps {
  audio: SessionAudio
  onTimeUpdate: (time: number) => void
  onPlayStateChange: (isPlaying: boolean) => void
  onDurationChange: (seconds: number) => void
  onSpeedChange: (rate: number) => void
  /** Observed by the page: once this element leaves the viewport the mini dock
   *  takes over, so the reader always has a transport within reach. */
  cardRef?: Ref<HTMLDivElement>
}

/**
 * The reader's audio card — the page's only `<audio>` element. It is the top of
 * the left column rather than a bar pinned to the window, so the transport sits
 * beside the words it plays; the mini dock is what covers the rest of the page.
 *
 * The handle is forwarded because seeking belongs to the transcript: clicking a
 * turn calls `seek` on it, and `SessionReaderPage` owns that wiring.
 */
export const AudioCard = forwardRef<AudioPlayerHandle, AudioCardProps>(function AudioCard(
  { audio, onTimeUpdate, onPlayStateChange, onDurationChange, onSpeedChange, cardRef },
  ref,
) {
  return (
    <div ref={cardRef} data-testid="reader-player">
      <AudioPlayer
        ref={ref}
        src={audio.streamUrl}
        isLoading={audio.isLoading}
        onError={audio.onError}
        onLoaded={audio.onLoaded}
        onTimeUpdate={onTimeUpdate}
        onPlayStateChange={onPlayStateChange}
        onDurationChange={onDurationChange}
        onSpeedChange={onSpeedChange}
      />
    </div>
  )
})
