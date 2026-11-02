import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Utterance } from '@/types/transcription'
import { SpeakerStrip } from './SpeakerStrip'

function turn(speaker: number, start: number, end: number): Utterance {
  return { id: start, speaker, start, end, text: 'x', words: [] }
}

const TURNS = [turn(0, 0, 30), turn(1, 36, 40), turn(0, 40, 60)]

function renderStrip(props: Partial<React.ComponentProps<typeof SpeakerStrip>> = {}) {
  const onSeek = props.onSeek ?? vi.fn()
  render(
    <SpeakerStrip
      utterances={TURNS}
      speakerMap={{ '0': 'Ana Vidal' }}
      duration={60}
      currentTime={0}
      {...props}
      onSeek={onSeek}
    />,
  )
  return { onSeek }
}

describe('SpeakerStrip', () => {
  it('is hidden from assistive technology, which gets the speaking-time shares instead', () => {
    renderStrip()
    expect(screen.getByTestId('speaker-strip')).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByText('Speaking time: Ana Vidal 93%, Speaker 2 7%')).toHaveClass('sr-only')
  })

  it('names each segment by speaker and start time, and seeks there on click', async () => {
    const user = userEvent.setup()
    const { onSeek } = renderStrip()
    const strip = screen.getByTestId('speaker-strip')
    const segment = strip.querySelector('button[title="Speaker 2 · 00:36"]') as HTMLElement
    expect(segment).not.toBeNull()
    // Out of the tab order: the slider and the transcript are the keyboard path.
    expect(segment).toHaveAttribute('tabindex', '-1')
    await user.click(segment)
    expect(onSeek).toHaveBeenCalledWith(36)
  })

  it('marks the playing turn', () => {
    renderStrip({ currentTime: 38 })
    const playing = screen.getByTestId('speaker-strip').querySelectorAll<HTMLElement>('[data-playing="true"]')
    expect(playing).toHaveLength(1)
    expect(playing[0]).toHaveClass('bg-coral-mark')
    expect(playing[0]!.style.left).toBe('60%')
    expect(playing[0]!.style.width).toBe(`${(4 / 60) * 100}%`)
  })

  it('marks nothing in a gap between turns', () => {
    renderStrip({ currentTime: 33 })
    expect(screen.getByTestId('speaker-strip').querySelector('[data-playing="true"]')).toBeNull()
  })

  describe('on a two-hour hearing, where turns merge', () => {
    // 720 alternating ten-second turns merge to one segment per lane at 320 px.
    const HEARING = Array.from({ length: 720 }, (_, i) => turn(i % 2, i * 10, i * 10 + 9.5))

    it('colours only the playing turn, not the merged lane', () => {
      renderStrip({ utterances: HEARING, duration: 7200, currentTime: 3605 })
      const strip = screen.getByTestId('speaker-strip')
      const playing = strip.querySelectorAll<HTMLElement>('[data-playing="true"]')
      expect(playing).toHaveLength(1)
      expect(parseFloat(playing[0]!.style.left)).toBeCloseTo(50, 5)
      expect(parseFloat(playing[0]!.style.width)).toBeLessThan(1)
      for (const segment of strip.querySelectorAll('button')) {
        expect(segment).not.toHaveClass('bg-coral-mark')
      }
    })

    it('seeks by where the click landed, not to the start of the hearing', () => {
      const { onSeek } = renderStrip({ utterances: HEARING, duration: 7200 })
      const segment = screen.getByTestId('speaker-strip').querySelector('button') as HTMLElement
      vi.spyOn(segment, 'getBoundingClientRect').mockReturnValue({ left: 100, width: 320 } as DOMRect)

      fireEvent.click(segment, { clientX: 260 })

      // Speaker 1's lane, halfway through: the turn at or before 3594.75 s.
      expect(onSeek).toHaveBeenCalledWith(3580)
    })
  })

  it('falls back to the last turn end while the duration is unknown', () => {
    renderStrip({ duration: 0 })
    expect(screen.getByTestId('speaker-strip')).toBeInTheDocument()
  })

  it('draws nothing for a session without turns', () => {
    renderStrip({ utterances: [] })
    expect(screen.queryByTestId('speaker-strip')).not.toBeInTheDocument()
  })
})
