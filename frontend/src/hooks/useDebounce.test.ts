import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDebounce } from './useDebounce'

describe('useDebounce', () => {
  it('returns initial value immediately', () => {
    const { result } = renderHook(() => useDebounce('hello', 300))
    expect(result.current).toBe('hello')
  })

  it('debounces value changes', () => {
    vi.useFakeTimers()
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: 'a' } },
    )

    expect(result.current).toBe('a')

    // Change value — should not update yet
    rerender({ value: 'ab' })
    expect(result.current).toBe('a')

    // Advance time past debounce delay
    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(result.current).toBe('ab')

    vi.useRealTimers()
  })

  it('resets timer on rapid changes', () => {
    vi.useFakeTimers()
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: '' } },
    )

    rerender({ value: 'm' })
    act(() => {
      vi.advanceTimersByTime(100)
    })
    rerender({ value: 'me' })
    act(() => {
      vi.advanceTimersByTime(100)
    })
    rerender({ value: 'mee' })
    act(() => {
      vi.advanceTimersByTime(100)
    })
    // Only 300ms total but timer was reset each time
    expect(result.current).toBe('')

    // Wait for debounce
    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(result.current).toBe('mee')

    vi.useRealTimers()
  })
})
