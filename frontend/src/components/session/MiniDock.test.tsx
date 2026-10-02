import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MiniDock, type MiniDockProps } from './MiniDock'

/** jsdom has no `PointerEvent`, so RTL would send a plain Event with no
 *  coordinates and Radix would read `clientX` as undefined. */
class TestPointerEvent extends MouseEvent {
  readonly pointerId: number
  constructor(type: string, init: PointerEventInit = {}) {
    super(type, init)
    this.pointerId = init.pointerId ?? 0
  }
}

/**
 * Radix drives the scrubber from real geometry and pointer capture, neither of
 * which jsdom provides. A 100 px-wide root with capture stubbed makes clientX a
 * percentage of the duration.
 */
function primeSliderRoot(): HTMLElement {
  // Radix wraps the thumb in a positioning span, so the root is not the
  // thumb's parent; the dock's own `flex-1` is on the root itself.
  const root = screen.getByRole('slider').closest<HTMLElement>('.flex-1')
  if (!root) throw new Error('no slider root')
  root.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: 100, height: 10, right: 100, bottom: 10, x: 0, y: 0 }) as DOMRect
  root.setPointerCapture = vi.fn()
  root.releasePointerCapture = vi.fn()
  root.hasPointerCapture = () => true
  return root
}

function renderDock(overrides: Partial<MiniDockProps> = {}) {
  const props: MiniDockProps = {
    isPlaying: false,
    currentTime: 754,
    duration: 1796,
    speed: 1,
    onToggle: vi.fn(),
    onSeek: vi.fn(),
    onSpeedChange: vi.fn(),
    onBackToTranscript: vi.fn(),
    ...overrides,
  }
  render(<MiniDock {...props} />)
  return props
}

describe('MiniDock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('PointerEvent', TestPointerEvent)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is a labelled region pinned to the bottom of the content column', () => {
    renderDock()
    const dock = screen.getByRole('region', { name: 'Audio player' })
    expect(dock).toHaveClass('fixed')
    expect(dock).toHaveClass('bottom-0')
    // Clear of the sidebar on desktop and of the floating activity tray. The
    // sidebar's width is a token, so this offset cannot drift from MainLayout.
    // Both start at `md`: the tray (z-50, w-80, right-4) covers the dock from
    // the moment the dock stops being full width, not from `lg`.
    expect(dock).toHaveClass('md:left-[var(--app-sidebar-w)]')
    expect(dock).toHaveClass('md:pr-[21rem]')
  })

  it('reads elapsed over total', () => {
    renderDock()
    expect(screen.getByText('12:34 / 29:56')).toBeInTheDocument()
  })

  it('offers Play while paused', () => {
    renderDock({ isPlaying: false })
    expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument()
  })

  it('offers Pause while playing', () => {
    renderDock({ isPlaying: true })
    expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument()
  })

  it('reports a play/pause press', async () => {
    const props = renderDock()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Play' }))
    expect(props.onToggle).toHaveBeenCalledTimes(1)
  })

  it('reports a scrub', () => {
    const props = renderDock()
    const slider = screen.getByRole('slider')

    slider.focus()
    fireEvent.keyDown(slider, { key: 'ArrowRight' })

    expect(props.onSeek).toHaveBeenCalled()
  })

  describe('dragging the scrubber', () => {
    // The page throttles `currentTime` to 4 Hz, so a scrubber rendered straight
    // from it drops most pointermove frames and snaps back under the pointer.
    // The dock shows the dragged value itself and moves the element only once.
    it('shows the dragged position and seeks once, on commit', () => {
      const props = renderDock({ currentTime: 0, duration: 1000 })
      const root = primeSliderRoot()

      fireEvent.pointerDown(root, { button: 0, pointerId: 1, clientX: 50 })
      expect(screen.getByText('08:20 / 16:40')).toBeInTheDocument()
      expect(props.onSeek).not.toHaveBeenCalled()

      fireEvent.pointerMove(root, { pointerId: 1, clientX: 80 })
      expect(screen.getByText('13:20 / 16:40')).toBeInTheDocument()
      expect(props.onSeek).not.toHaveBeenCalled()

      fireEvent.pointerUp(root, { pointerId: 1, clientX: 80 })
      expect(props.onSeek).toHaveBeenCalledTimes(1)
      expect(props.onSeek).toHaveBeenCalledWith(800)
    })

    it('hands the scrubber back to playback once the drag ends', () => {
      renderDock({ currentTime: 0, duration: 1000 })
      const root = primeSliderRoot()

      fireEvent.pointerDown(root, { button: 0, pointerId: 1, clientX: 50 })
      fireEvent.pointerUp(root, { pointerId: 1, clientX: 50 })

      // Back on the page's time, which has not moved in this test.
      expect(screen.getByText('00:00 / 16:40')).toBeInTheDocument()
    })
  })

  it('reports a speed change', async () => {
    const props = renderDock()
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox', { name: 'Playback speed' }))
    await user.click(screen.getByRole('option', { name: '1.5x' }))

    expect(props.onSpeedChange).toHaveBeenCalledWith(1.5)
  })

  it('offers the stored speed as a rung when it is not one of the defaults', () => {
    renderDock({ speed: 1.1 })
    expect(screen.getByRole('combobox', { name: 'Playback speed' })).toHaveTextContent('1.1x')
  })

  it('sends the reader back to the transcript', async () => {
    const props = renderDock()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Back to transcript' }))
    expect(props.onBackToTranscript).toHaveBeenCalledTimes(1)
  })
})
