import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { AudioAnalysisCard } from './AudioAnalysisCard'
import type { AudioAnalysis } from '@/types/media'

function buildAnalysis(overrides: Partial<AudioAnalysis> = {}): AudioAnalysis {
  return {
    trust_level: 'high',
    findings: [
      { check: 'format_match', severity: 'high', status: 'pass', summary: 'Format matches container' },
      { check: 'codec_valid', severity: 'high', status: 'fail', summary: 'Codec invalid' },
      { check: 'sample_rate', severity: 'medium', status: 'warn', summary: 'Sample rate unusual' },
      { check: 'bit_depth', severity: 'info', status: 'skip', summary: 'Bit depth not checked' },
    ],
    disclaimer: 'Automated checks only, not forensic proof.',
    ...overrides,
  }
}

/** The panel opens folded, so every content assertion starts by opening it. */
function expand() {
  fireEvent.click(screen.getByRole('button', { name: /Audio Integrity/ }))
}

describe('AudioAnalysisCard', () => {
  // Integrity is a reassurance the reader consults, not something they read on
  // the way to the transcript.
  it('starts collapsed to a single row naming the verdict', () => {
    render(<AudioAnalysisCard analysis={buildAnalysis()} fileHash="abc123" />)

    const row = screen.getByRole('button', { name: 'Audio Integrity High Trust' })
    expect(row).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Integrity details')).not.toBeInTheDocument()
    expect(screen.queryByText('Automated checks only, not forensic proof.')).not.toBeInTheDocument()
  })

  it('reveals the findings, hash and disclaimer once expanded', () => {
    render(<AudioAnalysisCard analysis={buildAnalysis()} fileHash="abc123" />)
    expand()

    expect(screen.getByRole('button', { name: /Audio Integrity/ })).toHaveAttribute(
      'aria-expanded',
      'true'
    )
    expect(screen.getByText('1 of 4 checks passed')).toBeInTheDocument()
    expect(screen.getByText('Integrity details')).toBeInTheDocument()
    expect(screen.getByText('File hash')).toBeInTheDocument()
    expect(screen.getByText('Automated checks only, not forensic proof.')).toBeInTheDocument()
  })

  it('folds again on a second click', () => {
    render(<AudioAnalysisCard analysis={buildAnalysis()} />)
    expand()
    expect(screen.getByText('Integrity details')).toBeInTheDocument()

    expand()
    expect(screen.queryByText('Integrity details')).not.toBeInTheDocument()
  })

  it('colors finding dots by status using semantic tokens', () => {
    render(<AudioAnalysisCard analysis={buildAnalysis()} />)
    expand()
    fireEvent.click(screen.getByText('Integrity details'))

    const passRow = screen.getByText('Format matches container').previousSibling as HTMLElement
    const failRow = screen.getByText('Codec invalid').previousSibling as HTMLElement
    const warnRow = screen.getByText('Sample rate unusual').previousSibling as HTMLElement
    const skipRow = screen.getByText('Bit depth not checked').previousSibling as HTMLElement

    expect(passRow.className).toContain('bg-success')
    expect(failRow.className).toContain('bg-destructive')
    expect(warnRow.className).toContain('bg-warning')
    expect(skipRow.className).toContain('bg-muted-foreground')
  })

  it('opens each detail on its own', () => {
    render(<AudioAnalysisCard analysis={buildAnalysis()} fileHash="abc123" />)
    expand()

    const details = screen.getByRole('button', { name: /Integrity details/ })
    const hash = screen.getByRole('button', { name: /File hash/ })
    expect(details).toHaveAttribute('aria-expanded', 'false')
    expect(hash).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(details)
    expect(details).toHaveAttribute('aria-expanded', 'true')
    expect(hash).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('abc123')).not.toBeInTheDocument()

    fireEvent.click(hash)
    expect(screen.getByText('abc123')).toBeInTheDocument()
    expect(screen.getByText('Codec invalid')).toBeInTheDocument()
  })

  it('renders high trust without a hardcoded green class', () => {
    const { container } = render(<AudioAnalysisCard analysis={buildAnalysis({ trust_level: 'high' })} />)
    expect(screen.getByText('High Trust')).toBeInTheDocument()
    expect(container.innerHTML).not.toContain('green-600')
  })

  it('falls back to the unknown verdict for an unrecognized trust level', () => {
    render(
      <AudioAnalysisCard
        analysis={buildAnalysis({ trust_level: 'nonsense' as AudioAnalysis['trust_level'] })}
      />
    )
    expect(screen.getByRole('button', { name: 'Audio Integrity Unknown' })).toBeInTheDocument()
  })
})
