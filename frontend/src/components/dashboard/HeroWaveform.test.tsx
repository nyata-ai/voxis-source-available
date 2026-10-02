import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { HeroWaveform } from './HeroWaveform'

describe('HeroWaveform', () => {
  it('renders an SVG with an accessible audio-signature label', () => {
    const { getByRole } = render(<HeroWaveform seed="abc" />)
    expect(getByRole('img', { name: /audio signature/i })).toBeInTheDocument()
  })

  it('renders exactly 60 bar rects', () => {
    const { container } = render(<HeroWaveform seed="abc" />)
    expect(container.querySelectorAll('rect.hero-waveform__bar').length).toBe(60)
  })

  it('renders the playhead overlay (aria-hidden)', () => {
    const { container } = render(<HeroWaveform seed="abc" />)
    const playhead = container.querySelector('.hero-waveform__playhead')
    expect(playhead).not.toBeNull()
    expect(playhead?.getAttribute('aria-hidden')).toBe('true')
  })

  it('produces the same bar geometry for the same seed', () => {
    const { container: first } = render(<HeroWaveform seed="seed-x" />)
    const { container: second } = render(<HeroWaveform seed="seed-x" />)
    const heightsA = Array.from(first.querySelectorAll('rect.hero-waveform__bar')).map((r) =>
      r.getAttribute('height')
    )
    const heightsB = Array.from(second.querySelectorAll('rect.hero-waveform__bar')).map((r) =>
      r.getAttribute('height')
    )
    expect(heightsA).toEqual(heightsB)
  })

  it('produces different bar geometry for different seeds', () => {
    const { container: first } = render(<HeroWaveform seed="seed-a" />)
    const { container: second } = render(<HeroWaveform seed="seed-b" />)
    const heightsA = Array.from(first.querySelectorAll('rect.hero-waveform__bar')).map((r) =>
      r.getAttribute('height')
    )
    const heightsB = Array.from(second.querySelectorAll('rect.hero-waveform__bar')).map((r) =>
      r.getAttribute('height')
    )
    expect(heightsA).not.toEqual(heightsB)
  })

  it('applies an extra className when supplied', () => {
    const { container } = render(<HeroWaveform seed="abc" className="extra-class" />)
    expect(container.querySelector('.hero-waveform.extra-class')).not.toBeNull()
  })
})
