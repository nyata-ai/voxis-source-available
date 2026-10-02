import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ActiveSessionTakeoverDialog } from './ActiveSessionTakeoverDialog'
import type { RecordingSession } from '@/types/recording'

const mockUseFormatters = vi.hoisted(() => vi.fn(() => ({
  date: (value: string) => `localized-date:${value}`,
})))

vi.mock('@/i18n/useFormatters', () => ({
  useFormatters: mockUseFormatters,
}))

function buildSession(overrides: Partial<RecordingSession> = {}): RecordingSession {
  return {
    id: 'sess-1',
    organization_id: 'org-1',
    user_id: 'user-1',
    status: 'recording',
    mime_type: 'audio/webm',
    total_duration: 0,
    chunk_count: 3,
    created_at: '2026-02-27T00:00:00Z',
    updated_at: '2026-02-27T00:01:00Z',
    ...overrides,
  }
}

describe('ActiveSessionTakeoverDialog', () => {
  it('renders nothing when there is no active session', () => {
    render(
      <ActiveSessionTakeoverDialog session={null} onTakeOver={vi.fn()} onLeave={vi.fn()} />,
    )

    expect(screen.queryByText('Active recording found')).not.toBeInTheDocument()
  })

  it('shows the session context and warns that taking over interrupts it', () => {
    render(
      <ActiveSessionTakeoverDialog
        session={buildSession()}
        onTakeOver={vi.fn()}
        onLeave={vi.fn()}
      />,
    )

    expect(screen.getByText('Active recording found')).toBeInTheDocument()
    expect(screen.getByText('localized-date:2026-02-27T00:00:00Z')).toBeInTheDocument()
    expect(screen.getByText('3 segment(s) uploaded so far')).toBeInTheDocument()
    expect(
      screen.getByText(/Taking over interrupts that session/),
    ).toBeInTheDocument()
  })

  it('omits the chunk count when nothing has been uploaded yet', () => {
    render(
      <ActiveSessionTakeoverDialog
        session={buildSession({ chunk_count: 0 })}
        onTakeOver={vi.fn()}
        onLeave={vi.fn()}
      />,
    )

    expect(screen.queryByText(/uploaded so far/)).not.toBeInTheDocument()
  })

  it('triggers take over and leave callbacks', async () => {
    const user = userEvent.setup()
    const onTakeOver = vi.fn()
    const onLeave = vi.fn()

    render(
      <ActiveSessionTakeoverDialog
        session={buildSession()}
        onTakeOver={onTakeOver}
        onLeave={onLeave}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Take over' }))
    await user.click(screen.getByRole('button', { name: 'Go to library' }))

    expect(onTakeOver).toHaveBeenCalledTimes(1)
    expect(onLeave).toHaveBeenCalledTimes(1)
  })

  it('disables both actions while a take over is in flight', () => {
    render(
      <ActiveSessionTakeoverDialog
        session={buildSession()}
        onTakeOver={vi.fn()}
        onLeave={vi.fn()}
        isBusy
      />,
    )

    expect(screen.getByRole('button', { name: 'Taking over...' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Go to library' })).toBeDisabled()
  })
})
