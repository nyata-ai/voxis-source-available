import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouteErrorRecovery } from './RouteErrorRecovery'
import { isChunkLoadError, RELOAD_GUARD_KEY } from './chunk-load-error'

const mockUseRouteError = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return { ...actual, useRouteError: () => mockUseRouteError() }
})

function chunkError(message: string): TypeError {
  return new TypeError(message)
}

const FIREFOX_MESSAGE =
  'error loading dynamically imported module: https://example.invalid/assets/LandingPage-Jh_fLIQb.js'
const CHROME_MESSAGE =
  'Failed to fetch dynamically imported module: https://example.invalid/assets/LandingPage-Jh_fLIQb.js'
const SAFARI_MESSAGE = 'Importing a module script failed.'

describe('isChunkLoadError', () => {
  it('recognises the three engine wordings', () => {
    expect(isChunkLoadError(chunkError(FIREFOX_MESSAGE))).toBe(true)
    expect(isChunkLoadError(chunkError(CHROME_MESSAGE))).toBe(true)
    expect(isChunkLoadError(chunkError(SAFARI_MESSAGE))).toBe(true)
  })

  it('rejects ordinary errors and non-errors', () => {
    expect(isChunkLoadError(new Error('boom'))).toBe(false)
    expect(isChunkLoadError('error loading dynamically imported module')).toBe(false)
    expect(isChunkLoadError(undefined)).toBe(false)
  })
})

describe('RouteErrorRecovery', () => {
  beforeEach(() => {
    sessionStorage.clear()
    mockUseRouteError.mockReset()
  })

  it('reloads once for a chunk-load error and arms the guard first', async () => {
    mockUseRouteError.mockReturnValue(chunkError(FIREFOX_MESSAGE))
    const reload = vi.fn()

    render(<RouteErrorRecovery reload={reload} />)

    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
    expect(sessionStorage.getItem(RELOAD_GUARD_KEY)).toBeTruthy()
    // While reloading, no error card competes with the incoming page.
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('recognises the Chrome and Safari wordings too', async () => {
    for (const message of [CHROME_MESSAGE, SAFARI_MESSAGE]) {
      sessionStorage.clear()
      mockUseRouteError.mockReturnValue(chunkError(message))
      const reload = vi.fn()
      const { unmount } = render(<RouteErrorRecovery reload={reload} />)
      await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
      unmount()
    }
  })

  it('does not reload again within the guard window; renders the fallback instead', async () => {
    sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now() - 5_000))
    mockUseRouteError.mockReturnValue(chunkError(FIREFOX_MESSAGE))
    const reload = vi.fn()

    render(<RouteErrorRecovery reload={reload} />)

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(reload).not.toHaveBeenCalled()
  })

  it('reloads again once the guard window has lapsed', async () => {
    sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now() - 60_000))
    mockUseRouteError.mockReturnValue(chunkError(FIREFOX_MESSAGE))
    const reload = vi.fn()

    render(<RouteErrorRecovery reload={reload} />)

    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1))
  })

  it('falls back without reloading when storage is unusable (cannot guard a loop)', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota')
    })
    try {
      mockUseRouteError.mockReturnValue(chunkError(FIREFOX_MESSAGE))
      const reload = vi.fn()

      render(<RouteErrorRecovery reload={reload} />)

      expect(await screen.findByRole('alert')).toBeInTheDocument()
      expect(reload).not.toHaveBeenCalled()
    } finally {
      setItem.mockRestore()
    }
  })

  it('renders the fallback immediately for a non-chunk error', () => {
    mockUseRouteError.mockReturnValue(new Error('boom'))
    const reload = vi.fn()

    render(<RouteErrorRecovery reload={reload} />)

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(reload).not.toHaveBeenCalled()
  })

  it('retry on the fallback triggers a reload', async () => {
    mockUseRouteError.mockReturnValue(new Error('boom'))
    const reload = vi.fn()
    const user = userEvent.setup()

    render(<RouteErrorRecovery reload={reload} />)

    await user.click(screen.getByRole('button', { name: /try again/i }))
    expect(reload).toHaveBeenCalledTimes(1)
  })
})
