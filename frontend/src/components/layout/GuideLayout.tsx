import { Suspense, useEffect, type ReactNode } from 'react'
import { setForceLightTheme } from '@/stores/theme'
import { PageLoader } from '@/components/ui/page-loader'

interface GuideLayoutProps {
  children: ReactNode
}

/**
 * Full-page layout for the /guide route. Public: the guide is static bundled
 * content that makes no API calls, so visitors can read it before signing in
 * (the landing page links here). No sidebar/header chrome (like
 * RecordingLayout). Always shown on the light theme — the guide
 * screenshots and layout are designed against the app's light editorial
 * system — and restores the user's actual theme preference on unmount.
 */
export function GuideLayout({ children }: GuideLayoutProps) {
  useEffect(() => {
    // A one-shot classList.remove is not enough: App's own mount effect calls
    // applyTheme() after this child effect and would re-add `dark`. The store
    // flag survives every later applyTheme()/setTheme() until unmount.
    setForceLightTheme(true)
    return () => setForceLightTheme(false)
  }, [])

  return <Suspense fallback={<PageLoader />}>{children}</Suspense>
}
