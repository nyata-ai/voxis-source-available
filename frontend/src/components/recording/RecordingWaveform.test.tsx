import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RecordingWaveform } from './RecordingWaveform'
import { loudnessFromTimeDomain } from './loudness'

describe('RecordingWaveform', () => {
  it('renders fallback waveform when analyserNode is null', () => {
    render(<RecordingWaveform analyserNode={null} isPaused={false} />)

    expect(screen.getByRole('img', { name: 'Audio waveform' })).toBeInTheDocument()
  })

  it('renders fallback waveform when paused without analyserNode', () => {
    render(<RecordingWaveform analyserNode={null} isPaused />)

    expect(screen.getByRole('img', { name: 'Audio waveform' })).toBeInTheDocument()
  })

  it('draws the microphone check at 40% ink', () => {
    render(<RecordingWaveform analyserNode={null} isPaused={false} tone="test" />)

    expect(screen.getByRole('img', { name: 'Audio waveform' })).toHaveClass('opacity-40')
  })
})

/** A frame whose RMS is the given level. */
function frameAt(dbfs: number): Float32Array {
  return new Float32Array(256).fill(10 ** (dbfs / 20))
}

describe('loudnessFromTimeDomain', () => {
  it('reads silence as zero', () => {
    expect(loudnessFromTimeDomain(new Float32Array(256))).toBe(0)
    expect(loudnessFromTimeDomain(new Float32Array(0))).toBe(0)
  })

  it('reads a full-scale signal as one', () => {
    const loud = Float32Array.from({ length: 256 }, (_, i) => (i % 2 === 0 ? -1 : 1))
    expect(loudnessFromTimeDomain(loud)).toBe(1)
  })

  // On byte samples these measured 0.21 at -90 dBFS and 0.30 from -70 to -50.
  it('tells a dead input from quiet speech', () => {
    expect(loudnessFromTimeDomain(frameAt(-90))).toBe(0)
    expect(loudnessFromTimeDomain(frameAt(-70))).toBe(0)
    expect(loudnessFromTimeDomain(frameAt(-50))).toBeCloseTo(0.2, 3)
    expect(loudnessFromTimeDomain(frameAt(-40))).toBeCloseTo(0.4, 3)
    expect(loudnessFromTimeDomain(frameAt(-20))).toBeCloseTo(0.8, 3)
  })
})
