import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useActivity } from '@/hooks/useActivity'
import { mediaKeys } from '@/hooks/useMedia'
import { transcriptionKeys } from '@/hooks/useTranscription'
import i18n from '@/i18n'
import type { ActivityItem, ActivityResponse } from '@/types/activity'
import { ActivityTray } from './ActivityTray'
import { MAX_REMEMBERED } from './useDismissedActivity'

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

function trayTree(queryClient: QueryClient) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ActivityTray />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function renderTray(queryClient = new QueryClient()) {
  return render(trayTree(queryClient))
}

describe('ActivityTray', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

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

    expect(screen.getByText('5 processing', { selector: 'span.font-semibold' })).toBeInTheDocument()
    expect(screen.getAllByTestId('activity-card')).toHaveLength(4)
    expect(screen.getByRole('link', { name: '+1 more' })).toHaveAttribute('href', '/library')
  })

  // Below `md` the cards fold into one 44px pill; the tray marks itself so the
  // reader's dock keeps clear of it only while it is on screen.
  it('folds into a pill on phones that opens the cards on tap', async () => {
    mockItems([activityItem('one'), activityItem('two')])
    const { container } = renderTray()

    expect(container.querySelector('aside')).toHaveAttribute('data-activity-tray')
    const pill = screen.getByRole('button', { name: /2 processing/ })
    expect(pill).toHaveClass('min-h-11', 'md:hidden')
    expect(pill).toHaveAttribute('aria-expanded', 'false')
    const cards = document.getElementById(pill.getAttribute('aria-controls') ?? '')
    expect(cards).toHaveClass('max-md:hidden')

    await userEvent.click(pill)
    expect(pill).toHaveAttribute('aria-expanded', 'true')
    expect(cards).not.toHaveClass('max-md:hidden')
  })

  // Below `md` the pill is the tray's only visible text and the only text its
  // live region can announce, so it has to tell the truth about the items.
  it('says what failed on the pill rather than "processing"', () => {
    mockItems([activityItem('one', 'failed')])
    renderTray()

    const label = screen.getByTestId('activity-pill-label')
    expect(label).toHaveTextContent('Recording couldn\'t be processed')
    expect(label).toHaveClass('text-destructive-inverse')
  })

  it('counts the other failures on the pill, ahead of anything still running', () => {
    mockItems([
      activityItem('one'),
      activityItem('two', 'failed'),
      { ...activityItem('three', 'failed'), kind: 'transcription' },
    ])
    renderTray()

    expect(screen.getByTestId('activity-pill-label')).toHaveTextContent('Recording couldn\'t be processed +1 more')
  })

  it('counts only running items as processing', () => {
    mockItems([activityItem('one'), activityItem('two', 'completed')])
    renderTray()

    expect(screen.getByTestId('activity-pill-label')).toHaveTextContent('1 processing')
  })

  it('reads Ready, not "processing", once everything has finished', () => {
    mockItems([activityItem('one', 'completed')])
    renderTray()

    expect(screen.getByTestId('activity-pill-label')).toHaveTextContent('Ready')
  })

  it('announces a failure through the live region when the pill text changes', () => {
    const queryClient = new QueryClient()
    mockItems([activityItem('one')])
    const { container, rerender } = renderTray(queryClient)
    const live = container.querySelector('[aria-live="polite"]')
    const label = screen.getByTestId('activity-pill-label')
    expect(live).toContainElement(label)
    expect(label).toHaveTextContent('1 processing')

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

    expect(live).toContainElement(screen.getByTestId('activity-pill-label'))
    expect(screen.getByTestId('activity-pill-label')).toHaveTextContent('Recording couldn\'t be processed')
  })

  it('keeps a failed item until it is dismissed, so a phone can still read the reason', async () => {
    vi.useFakeTimers()
    mockItems([{ ...activityItem('one', 'failed'), error_message: 'The file has no audio track' }])
    renderTray()

    act(() => {
      vi.advanceTimersByTime(60_000)
    })

    expect(screen.getByTestId('activity-error-message')).toHaveTextContent('The file has no audio track')
    expect(screen.getByTestId('activity-pill-label')).toBeInTheDocument()
  })

  it('uses the inverse surface tokens for the count header and overflow link', () => {
    mockItems(['one', 'two', 'three', 'four', 'five'].map((id) => activityItem(id)))
    renderTray()

    expect(screen.getByText('5 processing', { selector: 'span.font-semibold' }).parentElement).toHaveClass(
      'border-background/20',
      'bg-foreground',
      'text-background',
    )
    expect(screen.getByRole('link', { name: '+1 more' })).toHaveClass('text-background')
  })

  // The Library keeps its own cached list; without this it can still say
  // "pending" while the tray already says the job failed.
  it('refreshes the media and transcription lists when an item flips to failed', () => {
    const queryClient = new QueryClient()
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    mockItems([activityItem('one')])
    const { rerender } = renderTray(queryClient)
    expect(invalidate).not.toHaveBeenCalled()

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

  describe('dismissed failures', () => {
    const STORAGE_KEY = 'voxis-activity-dismissed'

    async function dismissOnlyCard() {
      await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    }

    /** A dismissed failure, then the page reloading. */
    async function dismissThenReload(ref = 'bad') {
      mockItems([activityItem(ref, 'failed')])
      const before = renderTray()
      await dismissOnlyCard()
      expect(screen.queryByTestId('activity-card')).not.toBeInTheDocument()
      before.unmount()
    }

    it('stays dismissed across a reload', async () => {
      await dismissThenReload()
      renderTray()
      expect(screen.queryByTestId('activity-card')).not.toBeInTheDocument()
    })

    it('survives the first render of a reload, before the feed has been read', async () => {
      await dismissThenReload()

      mockUseActivity.mockReturnValue({ data: undefined } as ReturnType<typeof useActivity>)
      renderTray().unmount()

      mockItems([activityItem('bad', 'failed')])
      renderTray()
      expect(screen.queryByTestId('activity-card')).not.toBeInTheDocument()
    })

    it('lets go of the red pill and shows the running count', async () => {
      mockItems([activityItem('bad', 'failed'), activityItem('running')])
      renderTray()
      expect(screen.getByTestId('activity-pill-label')).toHaveClass('text-destructive-inverse')

      await userEvent.click(screen.getAllByRole('button', { name: 'Dismiss' })[0]!)

      const pill = screen.getByTestId('activity-pill-label')
      expect(pill).not.toHaveClass('text-destructive-inverse')
      expect(pill).toHaveTextContent(i18n.t('common:activity.count', { count: 1 }))
    })

    it('still shows a failure nobody dismissed', async () => {
      await dismissThenReload()
      mockItems([activityItem('bad', 'failed'), activityItem('worse', 'failed')])
      renderTray()
      expect(screen.getByText('Activity worse')).toBeInTheDocument()
      expect(screen.queryByText('Activity bad')).not.toBeInTheDocument()
    })

    it('shows a job dismissed while running once it fails', async () => {
      mockItems([activityItem('job')])
      const queryClient = new QueryClient()
      const view = renderTray(queryClient)
      await dismissOnlyCard()
      expect(screen.queryByTestId('activity-card')).not.toBeInTheDocument()

      mockItems([activityItem('job', 'failed')])
      view.rerender(trayTree(queryClient))
      expect(screen.getByTestId('activity-card')).toBeInTheDocument()
    })

    it('shows a retried job again when it fails a second time', async () => {
      await dismissThenReload('job')

      mockItems([activityItem('job')])
      const retry = renderTray()
      expect(screen.getByTestId('activity-card')).toBeInTheDocument()
      retry.unmount()

      mockItems([activityItem('job', 'failed')])
      renderTray()
      expect(screen.getByTestId('activity-card')).toBeInTheDocument()
    })

    it('forgets a failure once it leaves the feed', async () => {
      await dismissThenReload()
      expect(window.sessionStorage.getItem(STORAGE_KEY)).toContain('bad')

      mockItems([])
      renderTray()
      expect(window.sessionStorage.getItem(STORAGE_KEY)).toBeNull()
    })

    it('reads a bounded list and ignores anything malformed', () => {
      const newest = `ref-${MAX_REMEMBERED + 4}`
      const many = Array.from({ length: MAX_REMEMBERED + 5 }, (_, i) => `ref-${i}`)
      window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(many))
      mockItems([activityItem('ref-0', 'failed'), activityItem(newest, 'failed')])
      const bounded = renderTray()
      // The oldest fell off the end; the newest is still remembered.
      expect(screen.getByText('Activity ref-0')).toBeInTheDocument()
      expect(screen.queryByText(`Activity ${newest}`)).not.toBeInTheDocument()
      bounded.unmount()

      window.sessionStorage.setItem(STORAGE_KEY, '{not json')
      mockItems([activityItem('bad', 'failed')])
      renderTray()
      expect(screen.getByTestId('activity-card')).toBeInTheDocument()
    })

    it('still dismisses when storage is blocked', async () => {
      const blocked = () => {
        throw new Error('blocked')
      }
      const spies = [
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(blocked),
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(blocked),
        vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(blocked),
      ]
      mockItems([activityItem('bad', 'failed')])
      renderTray()
      await dismissOnlyCard()
      expect(screen.queryByTestId('activity-card')).not.toBeInTheDocument()
      for (const spy of spies) spy.mockRestore()
    })
  })
})
