import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RecoverRecordingDialog } from './RecoverRecordingDialog'
import type { RecordingSession } from '@/types/recording'

const mockUseFormatters = vi.hoisted(() => vi.fn(() => ({
  date: (value: string) => `localized-date:${value}`,
})))

vi.mock('@/i18n/useFormatters', () => ({
  useFormatters: mockUseFormatters,
}))

function buildSession(id: string): RecordingSession {
  return {
    id,
    organization_id: 'org-1',
    user_id: 'user-1',
    status: 'interrupted',
    mime_type: 'audio/webm',
    total_duration: 0,
    created_at: '2026-02-27T00:00:00Z',
    updated_at: '2026-02-27T00:01:00Z',
    last_chunk_at: '2026-02-27T00:01:00Z',
  }
}

describe('RecoverRecordingDialog', () => {
  it('renders single-session view and triggers actions', async () => {
    const user = userEvent.setup()
    const onRecover = vi.fn()
    const onDiscard = vi.fn()

    render(
      <RecoverRecordingDialog
        open
        onOpenChange={vi.fn()}
        sessions={[buildSession('sess-1')]}
        onRecover={onRecover}
        onDiscard={onDiscard}
        isRecovering={false}
        isDiscarding={false}
      />,
    )

    expect(screen.getByText('Recording Interrupted')).toBeInTheDocument()
    expect(screen.getByText('localized-date:2026-02-27T00:00:00Z')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Recover' }))
    await user.click(screen.getByRole('button', { name: 'Discard' }))

    expect(onRecover).toHaveBeenCalledWith('sess-1')
    expect(onDiscard).toHaveBeenCalledWith('sess-1')
  })

  it('renders multi-session list', () => {
    render(
      <RecoverRecordingDialog
        open
        onOpenChange={vi.fn()}
        sessions={[buildSession('sess-1'), buildSession('sess-2')]}
        onRecover={vi.fn()}
        onDiscard={vi.fn()}
        isRecovering={false}
        isDiscarding={false}
      />,
    )

    expect(screen.getByText('Interrupted Recordings')).toBeInTheDocument()
    expect(screen.getAllByText('localized-date:2026-02-27T00:00:00Z')).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Recover' })).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Discard' })).toHaveLength(2)
  })
})
