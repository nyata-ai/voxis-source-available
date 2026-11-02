import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useLocation, useSearchParams } from 'react-router-dom'
import { Header } from './Header'
import { Sidebar } from './Sidebar'
import { AppFooter } from './AppFooter'
import { ConfidentialityStrip } from './ConfidentialityStrip'
import { ActivityTray } from '@/components/activity/ActivityTray'
import { CaptureModal } from '@/components/capture/CaptureModal'
import { useCaptureStore } from '@/stores/capture'
import { isCaptureTab, routes } from '@/config/routes'
import { cn } from '@/lib/utils'

export function MainLayout({ children }: { children: ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const captureOpen = useCaptureStore((state) => state.open)
  const openCapture = useCaptureStore((state) => state.openCapture)
  const [searchParams, setSearchParams] = useSearchParams()
  const showActivityTray = useLocation().pathname !== routes.dashboard.path
  const closeSidebar = useCallback(() => setSidebarOpen(false), [])
  useEffect(() => {
    const tab = searchParams.get('capture')
    if (!isCaptureTab(tab)) return
    openCapture(tab)
    const next = new URLSearchParams(searchParams)
    next.delete('capture')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams, openCapture])
  useEffect(() => {
    if (!sidebarOpen) return
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeSidebar()
    }
    document.addEventListener('keydown', close)
    return () => document.removeEventListener('keydown', close)
  }, [sidebarOpen, closeSidebar])
  return (
    // A column at least one dynamic viewport tall whose middle row grows, so
    // the footer sits at the bottom of short pages instead of mid-screen.
    <div className="relative flex min-h-dvh flex-col">
      <Header onMenuClick={() => setSidebarOpen(!sidebarOpen)} />
      <div className="flex flex-1">
        {/* Mobile sidebar overlay */}
        {sidebarOpen && (
          <div
            className="fixed inset-0 z-40 bg-background/80 backdrop-blur-sm md:hidden"
            onClick={closeSidebar}
            aria-hidden="true"
          />
        )}
        <Sidebar
          className={cn(
            // dvh, not vh: on phones 100vh runs under the browser's bottom
            // bar and hides the foot of the drawer.
            'fixed left-0 top-14 z-40 h-[calc(100dvh-3.5rem)] w-56 border-r bg-background transition-transform md:translate-x-0',
            sidebarOpen ? 'translate-x-0' : '-translate-x-full'
          )}
          onNavigate={closeSidebar}
        />
        <main className="min-w-0 flex-1 md:ml-56">
          <ConfidentialityStrip />
          <div className="px-6 py-8 lg:px-10">{children}</div>
        </main>
      </div>
      <AppFooter />
      {captureOpen && <CaptureModal />}
      {showActivityTray && <ActivityTray />}
    </div>
  )
}
