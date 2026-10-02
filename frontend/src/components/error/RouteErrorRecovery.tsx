import { useEffect, useState } from 'react'
import { useRouteError } from 'react-router-dom'
import { ErrorFallback } from './ErrorFallback'
import { armReloadGuard, isChunkLoadError } from './chunk-load-error'

/*
 * Root-route error boundary with one-shot recovery for failed lazy imports.
 *
 * A route chunk fetch can fail for two mundane reasons: on app routes the
 * keycloak-js check-sso full-page redirect can abort it mid-flight (the
 * landing page no longer probes the session, see src/lib/keycloak.ts), or a
 * deploy replaced the hashed assets an open tab still references. Both are
 * cured by a single reload: the redirect has settled by then, and the fresh
 * index.html references chunks that exist (immutable-cached if already
 * delivered). Without this boundary the router renders its built-in dead-end
 * "Unexpected Application Error!" page.
 */

interface RouteErrorRecoveryProps {
  reload?: () => void
}

export function RouteErrorRecovery({
  reload = () => window.location.reload(),
}: RouteErrorRecoveryProps) {
  const error = useRouteError()
  const [mode, setMode] = useState<'deciding' | 'fallback'>(() =>
    isChunkLoadError(error) ? 'deciding' : 'fallback'
  )

  useEffect(() => {
    if (mode !== 'deciding') return
    if (armReloadGuard()) {
      reload()
    } else {
      setMode('fallback')
    }
  }, [mode, reload])

  // Reload is on its way — paint nothing rather than flash an error card.
  if (mode === 'deciding') return null

  return (
    <ErrorFallback
      error={error instanceof Error ? error : new Error(String(error))}
      resetErrorBoundary={reload}
    />
  )
}
