import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { AudioPlayer, type AudioPlayerHandle } from './AudioPlayer'

// AudioPlayer reads the user's playback-speed preference. Mocking the hook
// keeps these tests provider-free — the query client is not what's under test.
const mockPreferences = vi.fn(() => ({ data: undefined as { playback_speed: number } | undefined }))
vi.mock('@/hooks/useSettings', () => ({
  usePreferences: () => mockPreferences(),
}))

/** Gives the media element a real duration, which jsdom never reports. */
function primeDuration(container: HTMLElement, seconds: number): HTMLAudioElement {
  const audio = container.querySelector('audio')
  if (!audio) throw new Error('no audio element rendered')
  Object.defineProperty(audio, 'duration', { value: seconds, configurable: true })
  fireEvent.loadedMetadata(audio)
  return audio
}

describe('AudioPlayer', () => {
  beforeEach(() => {
    mockPreferences.mockReturnValue({ data: undefined })
  })

  it('renders play button', () => {
    render(<AudioPlayer src="https://example.com/audio.mp3" />)
    expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
  })

  it('renders mute button', () => {
    render(<AudioPlayer src="https://example.com/audio.mp3" />)
    expect(screen.getByRole('button', { name: 'Mute' })).toBeInTheDocument()
  })

  it('renders nothing when no src', () => {
    render(<AudioPlayer src={null} />)
    expect(screen.queryByRole('button', { name: 'Play' })).not.toBeInTheDocument()
  })

  it('disables play when loading', () => {
    render(<AudioPlayer src="https://example.com/audio.mp3" isLoading />)
    expect(screen.getByRole('button', { name: 'Play' })).toBeDisabled()
  })

  it('renders time display', () => {
    render(<AudioPlayer src="https://example.com/audio.mp3" />)
    expect(screen.getByText('00:00 / 00:00')).toBeInTheDocument()
  })

  it('renders playback slider', () => {
    render(<AudioPlayer src="https://example.com/audio.mp3" />)
    expect(screen.getByRole('slider')).toBeInTheDocument()
  })

  it('calls onTimeUpdate when seek is invoked via ref', () => {
    const onTimeUpdate = vi.fn()
    const ref = { current: null as AudioPlayerHandle | null }

    render(
      <AudioPlayer
        ref={(r) => { ref.current = r }}
        src="https://example.com/audio.mp3"
        onTimeUpdate={onTimeUpdate}
      />,
    )

    act(() => {
      ref.current?.seek(42.5)
    })

    expect(onTimeUpdate).toHaveBeenCalledWith(42.5)
  })

  describe('skipping and speed', () => {
    it('skips backward by 5 seconds, clamped at zero', () => {
      const onTimeUpdate = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onTimeUpdate={onTimeUpdate}
        />,
      )
      primeDuration(container, 100)

      act(() => { ref.current?.seek(20) })
      fireEvent.click(screen.getByRole('button', { name: 'Back 5 seconds' }))
      expect(onTimeUpdate).toHaveBeenLastCalledWith(15)

      act(() => { ref.current?.seek(2) })
      fireEvent.click(screen.getByRole('button', { name: 'Back 5 seconds' }))
      expect(onTimeUpdate).toHaveBeenLastCalledWith(0)
    })

    it('skips forward by 5 seconds, clamped at the duration', () => {
      const onTimeUpdate = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onTimeUpdate={onTimeUpdate}
        />,
      )
      primeDuration(container, 100)

      act(() => { ref.current?.seek(20) })
      fireEvent.click(screen.getByRole('button', { name: 'Forward 5 seconds' }))
      expect(onTimeUpdate).toHaveBeenLastCalledWith(25)

      act(() => { ref.current?.seek(98) })
      fireEvent.click(screen.getByRole('button', { name: 'Forward 5 seconds' }))
      expect(onTimeUpdate).toHaveBeenLastCalledWith(100)
    })

    it('applies the chosen speed to the media element', async () => {
      const user = userEvent.setup()
      const { container } = render(
        <AudioPlayer src="https://example.com/audio.mp3" />,
      )
      const audio = container.querySelector('audio')!
      expect(audio.playbackRate).toBe(1)

      await user.click(screen.getByRole('combobox', { name: 'Playback speed' }))
      await user.click(screen.getByRole('option', { name: '1.5x' }))

      expect(audio.playbackRate).toBe(1.5)
    })

    it('takes its initial rate from the playback-speed preference', () => {
      mockPreferences.mockReturnValue({ data: { playback_speed: 1.25 } })
      const { container } = render(
        <AudioPlayer src="https://example.com/audio.mp3" />,
      )
      expect(container.querySelector('audio')!.playbackRate).toBe(1.25)
      expect(screen.getByRole('combobox', { name: 'Playback speed' })).toHaveTextContent('1.25x')
    })

    it('seek() via the ref reports the new position', () => {
      const onTimeUpdate = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onTimeUpdate={onTimeUpdate}
        />,
      )

      act(() => { ref.current?.seek(42.5) })
      expect(onTimeUpdate).toHaveBeenCalledWith(42.5)
    })
  })

  describe('the ink card', () => {
    it('renders the transport, scrubber, speed and mute', () => {
      const { container } = render(
        <AudioPlayer src="https://example.com/audio.mp3" />,
      )

      expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Back 5 seconds' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Forward 5 seconds' })).toBeInTheDocument()
      expect(screen.getByRole('combobox', { name: 'Playback speed' })).toHaveTextContent('1x speed')
      expect(screen.getByRole('button', { name: 'Mute' })).toBeInTheDocument()
      expect(screen.getByRole('slider')).toBeInTheDocument()
      expect(container.querySelector('.bg-ink')).not.toBeNull()
    })

    // `ring-ring` is near-black in the light theme — the colour of the card
    // itself, so the focus ring would be invisible on exactly this surface.
    it('gives the ink card its own visible focus ring', () => {
      render(<AudioPlayer src="https://example.com/audio.mp3" />)

      expect(screen.getByRole('button', { name: 'Play' })).toHaveClass('focus-visible:ring-white')
      expect(screen.getByRole('button', { name: 'Back 5 seconds' })).toHaveClass(
        'focus-visible:ring-white',
      )
      expect(screen.getByRole('button', { name: 'Mute' })).toHaveClass('focus-visible:ring-white')
    })

    it('reads elapsed over total in one line', () => {
      const { container } = render(
        <AudioPlayer src="https://example.com/audio.mp3" />,
      )
      primeDuration(container, 1796)
      expect(screen.getByText('00:00 / 29:56')).toBeInTheDocument()
    })
  })

  describe('handle and reporting', () => {
    beforeEach(() => {
      vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
      vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
    })

    // The element's own events own the state — the handle asks, it does not
    // assume. Without that, a rejected play() leaves a "playing" nothing can
    // correct.
    it('toggle() plays and pauses, following the element rather than guessing', () => {
      const onPlayStateChange = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onPlayStateChange={onPlayStateChange}
        />,
      )
      const audio = container.querySelector('audio')!

      act(() => { ref.current?.toggle() })
      expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
      // Nothing has changed yet: the element has not said it started.
      expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()

      act(() => { fireEvent.play(audio) })
      expect(onPlayStateChange).toHaveBeenLastCalledWith(true)
      expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument()

      act(() => { ref.current?.toggle() })
      expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled()

      act(() => { fireEvent.pause(audio) })
      expect(onPlayStateChange).toHaveBeenLastCalledWith(false)
      expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
    })

    // A play() the browser aborts (a second toggle, an expired signed URL)
    // rejects and fires no `pause` of its own.
    it('settles to paused when play() rejects', async () => {
      let reject!: (reason: Error) => void
      vi.spyOn(HTMLMediaElement.prototype, 'play').mockReturnValueOnce(
        new Promise<void>((_resolve, rej) => { reject = rej }),
      )
      const onPlayStateChange = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onPlayStateChange={onPlayStateChange}
        />,
      )
      const audio = container.querySelector('audio')!

      act(() => { ref.current?.toggle() })
      act(() => { fireEvent.play(audio) })
      expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument()

      await act(async () => { reject(new DOMException('interrupted', 'AbortError')) })

      expect(onPlayStateChange).toHaveBeenLastCalledWith(false)
      expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
    })

    it('setSpeed() applies a rate and reports it', () => {
      const onSpeedChange = vi.fn()
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
          onSpeedChange={onSpeedChange}
        />,
      )

      act(() => { ref.current?.setSpeed(1.5) })

      expect(container.querySelector('audio')!.playbackRate).toBe(1.5)
      expect(onSpeedChange).toHaveBeenLastCalledWith(1.5)
    })

    // A zero or negative rate would silence the element rather than fail loudly.
    it('setSpeed() ignores a rate that is not a positive number', () => {
      const ref = { current: null as AudioPlayerHandle | null }
      const { container } = render(
        <AudioPlayer
          ref={(r) => { ref.current = r }}
          src="https://example.com/audio.mp3"
        />,
      )

      act(() => { ref.current?.setSpeed(0) })
      act(() => { ref.current?.setSpeed(Number.NaN) })

      expect(container.querySelector('audio')!.playbackRate).toBe(1)
    })

    it('reports the duration once metadata lands', () => {
      const onDurationChange = vi.fn()
      const { container } = render(
        <AudioPlayer
          src="https://example.com/audio.mp3"
          onDurationChange={onDurationChange}
        />,
      )

      primeDuration(container, 1796)
      expect(onDurationChange).toHaveBeenLastCalledWith(1796)
    })

    // A live stream reports Infinity and a truncated header NaN. Either would
    // reach the card and the mini dock as "NaN:NaN".
    it('reports an unstatable duration as 0 rather than NaN or Infinity', () => {
      const onDurationChange = vi.fn()
      const { container } = render(
        <AudioPlayer
          src="https://example.com/audio.mp3"
          onDurationChange={onDurationChange}
        />,
      )

      primeDuration(container, 1796)
      expect(onDurationChange).toHaveBeenLastCalledWith(1796)

      primeDuration(container, Number.NaN)
      expect(onDurationChange).toHaveBeenLastCalledWith(0)
      expect(screen.getByText('00:00 / 00:00')).toBeInTheDocument()

      primeDuration(container, Number.POSITIVE_INFINITY)
      expect(onDurationChange).toHaveBeenLastCalledWith(0)
      expect(screen.getByText('00:00 / 00:00')).toBeInTheDocument()
    })
  })
})
