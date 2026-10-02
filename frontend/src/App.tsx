/* Voxis Source-Available frontend entry point. See the repository licensing files. */

import { useEffect, useSyncExternalStore } from 'react'
import { RouterProvider } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { ErrorBoundary } from 'react-error-boundary'
import { I18nextProvider } from 'react-i18next'
import { AuthProvider } from './contexts/AuthProvider'
import { ErrorFallback } from '@/components/error/ErrorFallback'
import { Toaster } from '@/components/ui/sonner'
import { queryClient } from '@/lib/query-client'
import { router } from './router'
import { useThemeStore } from './stores/theme'
import { useSyncLanguage } from '@/i18n/useSyncLanguage'
import i18n from './i18n'

/**
 * Applies the signed-in reader's saved UI language, everywhere.
 *
 * Mounted above the router rather than inside MainLayout, because detection
 * deliberately writes nothing to localStorage: without this the saved language
 * would reach only the authenticated shell, and the landing, login, and legal
 * routes would stay on the geo-detected guess
 * for good. Must sit inside AuthProvider — usePreferences gates on auth state.
 */
function LanguageSync() {
  const pathname = useSyncExternalStore(router.subscribe, () => router.state.location.pathname)
  useSyncLanguage(pathname)
  return null
}

function App() {
  useEffect(() => {
    useThemeStore.getState().applyTheme()
  }, [])

  return (
    <ErrorBoundary
      FallbackComponent={ErrorFallback}
      onReset={() => window.location.reload()}
      onError={(error, info) => {
        if (import.meta.env.DEV) {
          console.error('Application error:', error, info)
        }
      }}
    >
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <AuthProvider>
            <LanguageSync />
            <RouterProvider router={router} />
            <Toaster />
          </AuthProvider>
        </QueryClientProvider>
      </I18nextProvider>
    </ErrorBoundary>
  )
}

export default App
