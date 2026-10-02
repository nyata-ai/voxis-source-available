import { useEffect, useMemo, useState } from 'react'
import i18n from './index'
import { DEFAULT_LOCALE } from './config'
import { createFormatters } from './formatters'

function currentFormatterLocale(): string {
  return i18n.language || i18n.resolvedLanguage || DEFAULT_LOCALE
}

export function useFormatters() {
  const [locale, setLocale] = useState(currentFormatterLocale)

  useEffect(() => {
    const handleLanguageChanged = () => setLocale(currentFormatterLocale())
    i18n.on('languageChanged', handleLanguageChanged)
    return () => {
      i18n.off('languageChanged', handleLanguageChanged)
    }
  }, [])

  return useMemo(() => createFormatters(locale), [locale])
}
