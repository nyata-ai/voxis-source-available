import { Suspense, useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet } from 'react-router-dom'
import { useAuth } from '../../contexts/AuthContext'
import { MainLayout } from '../layout/MainLayout'
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card'
import { Button } from '../ui/button'
import { PageLoader } from '../ui/page-loader'
import { useAccessStore } from '@/stores/access'

interface ProtectedRouteProps {
  children: ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { t } = useTranslation('auth')
  const { isAuthenticated, isLoading, isLoggingOut, error, login, logout } = useAuth()
  const roleRequired = useAccessStore((s) => s.roleRequired)
  const loginInitiated = useRef(false)

  useEffect(() => {
    // Prevent multiple login redirects
    if (!isLoading && !isAuthenticated && !isLoggingOut && !error && !loginInitiated.current) {
      loginInitiated.current = true
      login()
    }
  }, [isLoading, isAuthenticated, isLoggingOut, error, login])

  // Reset the flag if auth state changes (e.g., user becomes authenticated)
  useEffect(() => {
    if (isAuthenticated) {
      loginInitiated.current = false
    }
  }, [isAuthenticated])

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent mx-auto mb-4" />
          <p className="text-muted-foreground">{t('protected.loading')}</p>
        </div>
      </div>
    )
  }

  if (isLoggingOut) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent mx-auto mb-4" />
          <p className="text-muted-foreground">{t('protected.signingOut')}</p>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="text-destructive">{t('protected.errorTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-muted-foreground">
              {t('protected.errorDescription')}
            </p>
            <Button onClick={() => window.location.reload()} className="w-full">
              {t('protected.tryAgain')}
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent mx-auto mb-4" />
          <p className="text-muted-foreground">{t('protected.redirecting')}</p>
        </div>
      </div>
    )
  }

  // Signed in, but the server refused this account the app role. Signing in
  // again cannot fix that, so offer sign-out rather than a login redirect.
  if (roleRequired) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md" role="alert">
          <CardHeader>
            <CardTitle>{t('protected.roleRequiredTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-muted-foreground">{t('protected.roleRequiredDescription')}</p>
            <Button onClick={() => void logout()} className="w-full">
              {t('layout.signOut')}
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return <>{children}</>
}

export function ProtectedLayout() {
  return (
    <ProtectedRoute>
      <MainLayout>
        <Suspense fallback={<PageLoader />}>
          <Outlet />
        </Suspense>
      </MainLayout>
    </ProtectedRoute>
  )
}
