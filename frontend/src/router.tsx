import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom'
import { lazy, Suspense, type ReactNode } from 'react'
import { RecordingLayout } from '@/components/layout/RecordingLayout'
import { GuideLayout } from '@/components/layout/GuideLayout'
import { LegacyListRedirect } from '@/components/layout/LegacyListRedirect'
import { ProtectedLayout } from '@/components/auth/ProtectedRoute'
import { AdminRoute } from '@/components/auth/AdminRoute'
import { RouteErrorRecovery } from '@/components/error/RouteErrorRecovery'
import { PageLoader } from '@/components/ui/page-loader'
import i18n from '@/i18n'

const DashboardPage = lazy(() =>
  import('@/pages/DashboardPage').then((m) => ({ default: m.DashboardPage }))
)
const RecordingPage = lazy(() =>
  import('@/pages/RecordingPage').then((m) => ({ default: m.RecordingPage }))
)
const GuidePage = lazy(() =>
  import('@/pages/guide/GuidePage').then((m) => ({ default: m.GuidePage }))
)
const MediaFileRoute = lazy(() =>
  import('@/pages/reader/MediaFileRoute').then((m) => ({ default: m.MediaFileRoute }))
)
const LibraryPage = lazy(() =>
  import('@/pages/LibraryPage').then((m) => ({ default: m.LibraryPage }))
)
const MediaSessionRoute = lazy(() =>
  import('@/pages/reader/MediaSessionRoute').then((m) => ({ default: m.MediaSessionRoute }))
)
const SummaryRedirect = lazy(() =>
  import('@/pages/reader/SummaryRedirect').then((m) => ({ default: m.SummaryRedirect }))
)
const SettingsPage = lazy(() =>
  import('@/pages/SettingsPage').then((m) => ({ default: m.SettingsPage }))
)
const AdminPage = lazy(() => import('@/pages/AdminPage').then((m) => ({ default: m.AdminPage })))
const LandingPage = lazy(() =>
  import('@/pages/LandingPage').then((m) => ({ default: m.LandingPage }))
)
const LoginPage = lazy(() => import('@/pages/LoginPage').then((m) => ({ default: m.LoginPage })))
const LegalPage = lazy(() => import('@/pages/LegalPage').then((m) => ({ default: m.LegalPage })))

const publicPage = (element: ReactNode) => <Suspense fallback={<PageLoader />}>{element}</Suspense>

const routes: RouteObject[] = [
  ...(['en', 'id'] as const).map((locale) => ({
    path: `/${locale}`,
    loader: async () => {
      await i18n.changeLanguage(locale)
      return null
    },
    element: publicPage(<LandingPage />),
  })),
  { path: '/', element: publicPage(<LandingPage />) },
  { path: '/login', element: publicPage(<LoginPage />) },
  { path: '/terms', element: publicPage(<LegalPage />) },
  { path: '/privacy', element: publicPage(<LegalPage />) },
  {
    path: '/record',
    element: (
      <RecordingLayout>
        <RecordingPage />
      </RecordingLayout>
    ),
  },
  {
    path: '/guide',
    element: (
      <GuideLayout>
        <GuidePage />
      </GuideLayout>
    ),
  },
  {
    element: <ProtectedLayout />,
    children: [
      { path: '/dashboard', element: <DashboardPage /> },
      { path: '/upload', element: <LegacyListRedirect to="/library" /> },
      { path: '/media', element: <LegacyListRedirect to="/library" /> },
      { path: '/library', element: <LibraryPage /> },
      { path: '/media/:id', element: <MediaFileRoute /> },
      { path: '/transcriptions', element: <LegacyListRedirect to="/library" /> },
      { path: '/transcriptions/:id', element: <MediaSessionRoute /> },
      { path: '/summaries', element: <LegacyListRedirect to="/library" /> },
      { path: '/summaries/:id', element: <SummaryRedirect /> },
      {
        path: '/admin',
        element: <AdminRoute />,
        children: [{ index: true, element: <AdminPage /> }],
      },
      { path: '/settings', element: <SettingsPage /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
]

export const router = createBrowserRouter([
  { errorElement: <RouteErrorRecovery />, children: routes },
])
