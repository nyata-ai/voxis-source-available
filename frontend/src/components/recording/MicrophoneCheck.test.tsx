import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { MicrophoneCheck } from './MicrophoneCheck'
import { MIC_CHECK_MS } from './useMicrophoneCheck'

const stopTrack = vi.fn()
const closeContext = vi.fn(() => Promise.resolve())
const getUserMedia = vi.fn()
/** What the fake microphone delivers, as an RMS amplitude; 0 is silence. */
let inputAmplitude = 0

class FakeAudioContext {
  state = 'running'
  createAnalyser() {
    return {
      fftSize: 0,
      getFloatTimeDomainData: (samples: Float32Array) => samples.fill(inputAmplitude),
    }
  }
  createMediaStreamSource() {
    return { connect: vi.fn() }
  }
  resume = vi.fn()
  close = closeContext
}

function fakeStream() {
  const stop = vi.fn()
  return { stream: { getTracks: () => [{ stop }] }, stop }
}

/** A `getUserMedia` answer the test releases by hand, to hold a start mid-flight. */
function deferredStream() {
  let resolve: (stream: unknown) => void = () => {}
  const promise = new Promise((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function testButton() {
  return screen.getByRole('button', { name: 'Test microphone' })
}

beforeEach(() => {
  vi.useFakeTimers()
  inputAmplitude = 0
  stopTrack.mockReset()
  closeContext.mockClear()
  getUserMedia.mockReset()
  getUserMedia.mockResolvedValue({ getTracks: () => [{ stop: stopTrack }] })
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: { getUserMedia },
  })
  vi.stubGlobal('AudioContext', FakeAudioContext)
  // jsdom has no 2D canvas; the tape simply skips drawing.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('MicrophoneCheck', () => {
  it('does not open the microphone until the user asks', () => {
    render(<MicrophoneCheck deviceId="mic-1" />)

    expect(getUserMedia).not.toHaveBeenCalled()
    expect(testButton()).toBeEnabled()
  })

  it('listens on the chosen device and lets go of it after ten seconds', async () => {
    render(<MicrophoneCheck deviceId="mic-1" />)

    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(getUserMedia).toHaveBeenCalledWith({ audio: { deviceId: { exact: 'mic-1' } } })
    expect(screen.getByRole('button', { name: 'Listening…' })).toHaveAttribute('aria-disabled', 'true')

    await act(async () => {
      vi.advanceTimersByTime(MIC_CHECK_MS)
    })

    expect(stopTrack).toHaveBeenCalled()
    expect(closeContext).toHaveBeenCalled()
    // The fake microphone only ever delivered silence.
    expect(screen.getByRole('status')).toHaveTextContent('No sound yet. Check the microphone.')
    expect(testButton()).not.toHaveAttribute('aria-disabled')
  })

  it('releases the microphone when the page moves on mid-check', async () => {
    const { unmount } = render(<MicrophoneCheck deviceId="" />)

    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(getUserMedia).toHaveBeenCalledWith({ audio: true })

    unmount()
    expect(stopTrack).toHaveBeenCalled()
  })

  it('lets go of the microphone and clears its result when the page starts recording', async () => {
    const { rerender } = render(<MicrophoneCheck deviceId="mic-1" />)
    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(stopTrack).not.toHaveBeenCalled()

    rerender(<MicrophoneCheck deviceId="mic-1" disabled />)

    expect(stopTrack).toHaveBeenCalledTimes(1)
    expect(closeContext).toHaveBeenCalledTimes(1)
    expect(testButton()).toBeDisabled()
    expect(screen.getByRole('status')).toHaveTextContent('')
  })

  it('stops a stream still on its way when the page starts recording', async () => {
    const pending = deferredStream()
    getUserMedia.mockReturnValueOnce(pending.promise)
    const { rerender } = render(<MicrophoneCheck deviceId="mic-1" />)
    await act(async () => {
      fireEvent.click(testButton())
    })

    rerender(<MicrophoneCheck deviceId="mic-1" disabled />)
    const late = fakeStream()
    await act(async () => {
      pending.resolve(late.stream)
    })

    expect(late.stop).toHaveBeenCalledTimes(1)
    expect(closeContext).not.toHaveBeenCalled()
    expect(testButton()).toBeDisabled()
  })

  it('cannot be started while the page is starting, and works again if the start gives up', async () => {
    const { rerender } = render(<MicrophoneCheck deviceId="mic-1" disabled />)
    fireEvent.click(testButton())
    expect(getUserMedia).not.toHaveBeenCalled()

    rerender(<MicrophoneCheck deviceId="mic-1" />)
    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(getUserMedia).toHaveBeenCalledTimes(1)
  })

  it('says so when the browser refuses the microphone', async () => {
    getUserMedia.mockRejectedValue(new DOMException('denied', 'NotAllowedError'))
    render(<MicrophoneCheck deviceId="mic-1" />)

    await act(async () => {
      fireEvent.click(testButton())
    })

    expect(screen.getByRole('status')).toHaveTextContent('Microphone access denied.')
  })

  it('opens one microphone on a double click', async () => {
    const pending = deferredStream()
    getUserMedia.mockReturnValueOnce(pending.promise)
    const { unmount } = render(<MicrophoneCheck deviceId="mic-1" />)

    const button = testButton()
    await act(async () => {
      fireEvent.click(button)
    })
    await act(async () => {
      fireEvent.click(button)
    })
    expect(getUserMedia).toHaveBeenCalledTimes(1)

    const { stream, stop } = fakeStream()
    await act(async () => {
      pending.resolve(stream)
    })
    unmount()
    expect(stop).toHaveBeenCalledTimes(1)
  })

  it('stops the older stream when two starts land before a render', async () => {
    const first = deferredStream()
    const second = deferredStream()
    getUserMedia.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const { unmount } = render(<MicrophoneCheck deviceId="mic-1" />)

    const button = testButton()
    await act(async () => {
      // One batch: the second click still sees the idle handler.
      fireEvent.click(button)
      fireEvent.click(button)
    })
    expect(getUserMedia).toHaveBeenCalledTimes(2)

    const older = fakeStream()
    const newer = fakeStream()
    await act(async () => {
      first.resolve(older.stream)
      second.resolve(newer.stream)
    })
    expect(older.stop).toHaveBeenCalledTimes(1)
    expect(newer.stop).not.toHaveBeenCalled()

    unmount()
    expect(newer.stop).toHaveBeenCalledTimes(1)
  })

  it('stops a stream that arrives after the page has moved on', async () => {
    const pending = deferredStream()
    getUserMedia.mockReturnValueOnce(pending.promise)
    const { unmount } = render(<MicrophoneCheck deviceId="mic-1" />)

    await act(async () => {
      fireEvent.click(testButton())
    })
    unmount()

    const { stream, stop } = fakeStream()
    await act(async () => {
      pending.resolve(stream)
    })
    expect(stop).toHaveBeenCalledTimes(1)
  })

  it('stops the stream when the audio context cannot start', async () => {
    const { stream, stop } = fakeStream()
    getUserMedia.mockResolvedValueOnce(stream)
    vi.stubGlobal(
      'AudioContext',
      class {
        constructor() {
          throw new Error('no audio')
        }
      },
    )
    render(<MicrophoneCheck deviceId="mic-1" />)

    await act(async () => {
      fireEvent.click(testButton())
    })

    expect(stop).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('status')).toHaveTextContent('Microphone access denied.')
  })

  it('shows it is busy before the browser answers, and keeps keyboard focus', async () => {
    getUserMedia.mockReturnValueOnce(deferredStream().promise)
    render(<MicrophoneCheck deviceId="mic-1" />)

    const button = testButton()
    button.focus()
    await act(async () => {
      fireEvent.click(button)
    })

    const busy = screen.getByRole('button', { name: 'Listening…' })
    expect(busy).toHaveAttribute('aria-disabled', 'true')
    expect(busy).not.toBeDisabled()
    expect(busy).toHaveFocus()
  })

  it('lets go of the old microphone and clears the result when the device changes', async () => {
    inputAmplitude = 0.1
    const { rerender } = render(<MicrophoneCheck deviceId="mic-1" />)

    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(screen.getByRole('status')).toHaveTextContent('We can hear you')
    expect(stopTrack).not.toHaveBeenCalled()

    rerender(<MicrophoneCheck deviceId="mic-2" />)

    expect(stopTrack).toHaveBeenCalledTimes(1)
    expect(getUserMedia).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('status')).toBeEmptyDOMElement()
    expect(testButton()).not.toHaveAttribute('aria-disabled')
  })

  it('hears speech-level input but not a quiet room', async () => {
    const { rerender } = render(<MicrophoneCheck deviceId="mic-1" />)

    inputAmplitude = 10 ** (-55 / 20)
    await act(async () => {
      fireEvent.click(testButton())
    })
    await act(async () => {
      vi.advanceTimersByTime(MIC_CHECK_MS)
    })
    expect(screen.getByRole('status')).toHaveTextContent('No sound yet. Check the microphone.')

    rerender(<MicrophoneCheck deviceId="mic-2" />)
    inputAmplitude = 10 ** (-30 / 20)
    await act(async () => {
      fireEvent.click(testButton())
    })
    expect(screen.getByRole('status')).toHaveTextContent('We can hear you')
  })
})
