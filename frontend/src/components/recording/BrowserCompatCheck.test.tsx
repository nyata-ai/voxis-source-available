import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BrowserCompatCheck } from './BrowserCompatCheck'

// Mock negotiateCodec from useMediaRecorder
vi.mock('@/hooks/useMediaRecorder', () => ({
  negotiateCodec: vi.fn(),
}))

describe('BrowserCompatCheck', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders children when browser is fully supported', async () => {
    const { negotiateCodec } = await import('@/hooks/useMediaRecorder')
    vi.mocked(negotiateCodec).mockReturnValue({
      supported: true,
      mimeType: 'audio/webm',
      codecString: 'audio/webm;codecs=opus',
    })

    // Ensure MediaRecorder and getUserMedia exist
    vi.stubGlobal('MediaRecorder', class {})
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn() },
      writable: true,
      configurable: true,
    })

    render(
      <BrowserCompatCheck>
        <div data-testid="child">Recording UI</div>
      </BrowserCompatCheck>,
    )

    expect(screen.getByTestId('child')).toBeInTheDocument()
    expect(screen.queryByText('Browser Not Supported')).not.toBeInTheDocument()
  })

  it('shows unsupported message when MediaRecorder is undefined', async () => {
    vi.stubGlobal('MediaRecorder', undefined)

    render(
      <BrowserCompatCheck>
        <div data-testid="child">Recording UI</div>
      </BrowserCompatCheck>,
    )

    expect(screen.getByText('Browser Not Supported')).toBeInTheDocument()
    expect(screen.queryByTestId('child')).not.toBeInTheDocument()
  })

  it('shows unsupported message when no codec is supported', async () => {
    const { negotiateCodec } = await import('@/hooks/useMediaRecorder')
    vi.mocked(negotiateCodec).mockReturnValue({
      supported: false,
      mimeType: null,
      codecString: null,
    })

    vi.stubGlobal('MediaRecorder', class {})
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn() },
      writable: true,
      configurable: true,
    })

    render(
      <BrowserCompatCheck>
        <div data-testid="child">Recording UI</div>
      </BrowserCompatCheck>,
    )

    expect(screen.getByText('Browser Not Supported')).toBeInTheDocument()
    expect(screen.queryByTestId('child')).not.toBeInTheDocument()
  })

  it('unsupported message has role="alert"', async () => {
    vi.stubGlobal('MediaRecorder', undefined)

    render(
      <BrowserCompatCheck>
        <div>child</div>
      </BrowserCompatCheck>,
    )

    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('mentions supported browsers in the message', async () => {
    vi.stubGlobal('MediaRecorder', undefined)

    render(
      <BrowserCompatCheck>
        <div>child</div>
      </BrowserCompatCheck>,
    )

    const message = screen.getByText(/Chrome, Firefox, Edge, or Safari/)
    expect(message).toBeInTheDocument()
  })
})
