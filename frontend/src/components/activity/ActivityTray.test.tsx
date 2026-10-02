import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useActivity } from '@/hooks/useActivity'
import { mediaKeys } from '@/hooks/useMedia'
import { transcriptionKeys } from '@/hooks/useTranscription'
import i18n from '@/i18n'
import type { ActivityItem, ActivityResponse } from '@/types/activity'
import { ActivityTray } from './ActivityTray'

vi.mock('@/hooks/useActivity', () => ({
  useActivity: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn() },
}))

const mockUseActivity = vi.mocked(useActivity)

function activityItem(ref_id: string, status: ActivityItem['status'] = 'in_progress'): ActivityItem {
  return {
    kind: 'recording',
    ref_id,
    title: `Activity ${ref_id}`,
    stage: status === 'completed' ? 'ready' : 'stitching',
    status,
    link: status === 'completed' ? `/media/${ref_id}` : '',
    started_at: '2026-07-09T05:58:00Z',
  }
}

function mockItems(items: ActivityItem[]) {
  mockUseActivity.mockReturnValue({
    data: { items } satisfies ActivityResponse,
    isLoading: false,
    isError: false,
  } as ReturnType<typeof useActivity>)
}

function renderTray(queryClient = new QueryClient()) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ActivityTray />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('ActivityTray', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('renders nothing when there are no activity items', () => {
    mockItems([])
    const { container } = renderTray()
    expect(container.firstChild).toBeNull()
  })

  it('renders one card for one in-progress item', () => {
    mockItems([activityItem('one')])
    renderTray()

    expect(screen.getByText('Activity one')).toBeInTheDocument()
    expect(screen.getAllByTestId('activity-card')).toHaveLength(1)
  })

  it('renders a compact count header, capped cards, and media link for overflow', () => {
    mockItems(['one', 'two', 'three', 'four', 'five'].map((id) => activityItem(id)))
    renderTray()

    expect(screen.getByText('5 processing')).toBeInTheDocument()
    expect(screen.getAllByTestId('activity-card')).toHaveLength(4)
    expect(screen.getByRole('link', { name: '+1 more' })).toHaveAttribute('href', '/library')
  })

  it('uses the inverse surface tokens for the count header and overflow link', () => {
    mockItems(['one', 'two', 'three', 'four', 'five'].map((id) => activityItem(id)))
    renderTray()

    expect(screen.getByText('5 processing').parentElement).toHaveClass(
      'border-background/20',
      'bg-foreground',
      'text-background',
    )
    expect(screen.getByRole('link', { name: '+1 more' })).toHaveClass('text-background')
  })

  it('refreshes cached lists when an item flips to failed', () => {
    const queryClient = new QueryClient()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    mockItems([activityItem('one')])
    const { rerender } = renderTray(queryClient)

    mockItems([activityItem('one', 'failed')])
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <ActivityTray />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    expect(invalidate).toHaveBeenCalledWith({ queryKey: mediaKeys.all })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: transcriptionKeys.all })
  })

  it('manually dismisses a card by ref id', async () => {
    const user = userEvent.setup()
    mockItems([activityItem('one', 'completed')])
    renderTray()

    await user.click(screen.getByRole('button', { name: 'Dismiss' }))

    expect(screen.queryByText('Activity one')).not.toBeInTheDocument()
  })

  it('auto-dismisses terminal cards after about 10 seconds', () => {
    vi.useFakeTimers()
    mockItems([activityItem('done', 'completed')])
    renderTray()

    expect(screen.getByText('Activity done')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(10_000)
    })

    expect(screen.queryByText('Activity done')).not.toBeInTheDocument()
  })

  it('does not reset an existing terminal card timer when another item appears', () => {
    vi.useFakeTimers()
    mockItems([activityItem('one', 'completed')])
    const { rerender } = renderTray()

    act(() => {
      vi.advanceTimersByTime(5_000)
    })

    mockItems([activityItem('one', 'completed'), activityItem('two', 'completed')])
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ActivityTray />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    act(() => {
      vi.advanceTimersByTime(5_000)
    })

    expect(screen.queryByText('Activity one')).not.toBeInTheDocument()
    expect(screen.getByText('Activity two')).toBeInTheDocument()
  })

  it('prunes dismissed refs once they disappear from the activity feed', async () => {
    const user = userEvent.setup()
    mockItems([activityItem('one', 'completed')])
    const { rerender } = renderTray()

    await user.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByText('Activity one')).not.toBeInTheDocument()

    mockItems([])
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ActivityTray />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    mockItems([activityItem('one', 'completed')])
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ActivityTray />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>,
    )

    expect(screen.getByText('Activity one')).toBeInTheDocument()
  })
})
