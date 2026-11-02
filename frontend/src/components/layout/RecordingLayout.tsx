import { Suspense, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { Button } from '@/components/ui/button'
import { PageLoader } from '@/components/ui/page-loader'

interface RecordingLayoutProps {
  children: ReactNode
}

/**
 * Full-page layout for the recording screen.
 * JWT-protected, no sidebar — the app header's height and wordmark, and a
 * Cancel that returns to wherever the user came from.
 */
export function RecordingLayout({ children }: RecordingLayoutProps) {
  return (
    <ProtectedRoute>
      <RecordingLayoutContent>{children}</RecordingLayoutContent>
    </ProtectedRoute>
  )
}

function RecordingLayoutContent({ children }: RecordingLayoutProps) {
  const { t } = useTranslation('recording')
  const navigate = useNavigate()
  // React Router marks the first entry of a session `default`: there is no
  // in-app page behind it (a bookmark, a new tab), so Back would leave the app.
  const cameFromApp = useLocation().key !== 'default'

  function handleCancel() {
    if (cameFromApp) navigate(-1)
    else navigate('/library')
  }

  return (
    <div className="flex min-h-dvh flex-col bg-background">
      <header className="flex h-14 shrink-0 items-center justify-between border-b px-3 sm:px-4 lg:px-6">
        <Link to="/dashboard" className="flex items-center pl-1 pr-3">
          <span className="text-sm font-semibold uppercase tracking-[0.18em]">{t('layout.brand')}</span>
        </Link>
        <Button variant="ghost" size="sm" onClick={handleCancel} className="gap-1.5 max-sm:h-11">
          <X className="h-4 w-4" aria-hidden="true" />
          {t('layout.cancel')}
        </Button>
      </header>
      <main className="flex flex-1 flex-col">
        <Suspense fallback={<PageLoader />}>{children}</Suspense>
      </main>
    </div>
  )
}
