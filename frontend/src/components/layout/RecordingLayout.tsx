import { Suspense, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { PageLoader } from '@/components/ui/page-loader'

interface RecordingLayoutProps {
  children: ReactNode
}

/**
 * Full-page layout for the recording screen.
 * JWT-protected, no sidebar or header — just the Voxis logo and cancel link.
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

  return (
    <div className="min-h-screen bg-background">
      <header className="flex items-center gap-4 px-6 py-4 border-b">
        <Link to="/library" className="text-lg font-bold tracking-tight">
          {t('layout.brand')}
        </Link>
        <Link
          to="/library"
          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          {t('layout.cancel')}
        </Link>
      </header>
      <main className="flex-1">
        <Suspense fallback={<PageLoader />}>{children}</Suspense>
      </main>
    </div>
  )
}
