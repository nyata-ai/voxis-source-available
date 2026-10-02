import { describe, expect, it } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import i18n from './index'
import { NAMESPACES } from './config'
import { useFormatters } from './useFormatters'

describe('useFormatters', () => {
  it('recomputes formatters when the i18n language changes', async () => {
    const originalSupportedLngs = i18n.options.supportedLngs
    const supportedLngs = Array.isArray(originalSupportedLngs) ? originalSupportedLngs : []
    i18n.options.supportedLngs = [...supportedLngs, 'de']
    NAMESPACES.forEach((namespace) => {
      i18n.addResourceBundle('de', namespace, {}, true, true)
    })

    await i18n.changeLanguage('en')

    const { result } = renderHook(() => useFormatters())
    expect(result.current.number(1234.5)).toBe('1,234.5')

    await act(async () => {
      await i18n.changeLanguage('de')
    })

    await waitFor(() => {
      expect(result.current.number(1234.5)).toBe('1.234,5')
    })

    await act(async () => {
      await i18n.changeLanguage('en')
    })

    i18n.options.supportedLngs = originalSupportedLngs
  })
})
