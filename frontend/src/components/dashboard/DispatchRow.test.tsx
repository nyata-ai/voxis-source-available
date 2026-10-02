import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DispatchRow } from './DispatchRow'
import type { DashboardStats } from '@/types/dashboard'

const baseStats: DashboardStats = {
  total_media: 0,
  processing_count: 0,
  encrypting_count: 0,
  transcribing_count: 0,
  completed_transcriptions: 0,
  completed_this_month: 0,
  completed_last_month: 0,
  seconds_this_month: 0,
  last_completed_at: null,
}

describe('DispatchRow', () => {
  beforeEach(() => {
    // Pin "now" to 2026-05-23 so month-range and lede text are deterministic.
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-23T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders the two stats eyebrows', () => {
    render(<DispatchRow stats={baseStats} />)
    expect(screen.getByText('Volume')).toBeInTheDocument()
    expect(screen.getByText('Output')).toBeInTheDocument()
  })

  it('no longer carries an In-motion cell — the Desk band reports in-flight work', () => {
    render(
      <DispatchRow
        stats={{
          ...baseStats,
          processing_count: 3,
          transcribing_count: 2,
          encrypting_count: 1,
        }}
      />,
    )
    expect(screen.queryByText('In motion')).not.toBeInTheDocument()
    expect(screen.queryByText('2 transcribing · 1 encrypting')).not.toBeInTheDocument()
    expect(screen.queryByText(/all clear/)).not.toBeInTheDocument()
  })

  it('shows em-dash for zero volume and "fresh month" lede', () => {
    render(<DispatchRow stats={baseStats} />)
    // Em-dash appears in both Volume and Output cells when both are zero.
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
    expect(screen.getByText('fresh month')).toBeInTheDocument()
  })

  it('formats hours with one decimal when >= 1h', () => {
    render(<DispatchRow stats={{ ...baseStats, seconds_this_month: 98640 }} />)
    expect(screen.getByText('27.4')).toBeInTheDocument()
    expect(screen.getByText('hours')).toBeInTheDocument()
  })

  it('switches to minutes when under an hour', () => {
    render(<DispatchRow stats={{ ...baseStats, seconds_this_month: 1800 }} />)
    expect(screen.getByText('30')).toBeInTheDocument()
    expect(screen.getByText('minutes')).toBeInTheDocument()
  })

  it('renders comparison lede when both months have completions', () => {
    render(
      <DispatchRow
        stats={{
          ...baseStats,
          completed_this_month: 18,
          completed_last_month: 15,
        }}
      />,
    )
    expect(screen.getByText('18')).toBeInTheDocument()
    expect(screen.getByText('+3 vs April')).toBeInTheDocument()
  })

  it('shows "first this month" when last month was zero', () => {
    render(
      <DispatchRow stats={{ ...baseStats, completed_this_month: 4, completed_last_month: 0 }} />,
    )
    expect(screen.getByText('first this month')).toBeInTheDocument()
  })

  it('wraps previous-month name to December when current month is January', () => {
    vi.setSystemTime(new Date('2026-01-12T09:00:00Z'))
    render(
      <DispatchRow
        stats={{
          ...baseStats,
          completed_this_month: 7,
          completed_last_month: 4,
        }}
      />,
    )
    expect(screen.getByText('+3 vs December')).toBeInTheDocument()
  })
})
