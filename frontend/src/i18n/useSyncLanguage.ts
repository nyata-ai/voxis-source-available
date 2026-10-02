import { useEffect } from 'react'
import { usePreferences } from '@/hooks/useSettings'
import { applyServerLanguage, requestedLanguage } from './language'
import { landingLocale } from './landing-locale'

export function useSyncLanguage(pathname = window.location.pathname) {
  const { data } = usePreferences()
  useEffect(() => {
    if (landingLocale(pathname)) return
    const pref = data?.ui_language
    // requestedLanguage, NOT currentLanguage: this effect runs once per
    // preference value, so a skip here is permanent. During the load window of a
    // detected non-English boot currentLanguage() still reads 'en', so a stored
    // 'en' — a real choice the reader made on another device — would look
    // already-applied and be dropped for the rest of the session.
    if (pref && pref !== requestedLanguage()) void applyServerLanguage(pref)
  }, [data?.ui_language, pathname])
}
