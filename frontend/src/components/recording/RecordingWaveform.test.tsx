import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RecordingWaveform } from './RecordingWaveform'

describe('RecordingWaveform', () => {
  it('renders fallback waveform when analyserNode is null', () => {
    render(<RecordingWaveform analyserNode={null} isPaused={false} />)

    expect(screen.getByRole('img', { name: 'Audio waveform' })).toBeInTheDocument()
  })

  it('renders fallback waveform when paused without analyserNode', () => {
    render(<RecordingWaveform analyserNode={null} isPaused />)

    expect(screen.getByRole('img', { name: 'Audio waveform' })).toBeInTheDocument()
  })
})
