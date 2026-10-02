import { Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { getVisibleNavGroups, type RouteConfig } from '@/config/routes'
import { useAdmin } from '@/hooks/useAdmin'
import { useCaptureStore } from '@/stores/capture'

export function Sidebar({
  className,
  onNavigate,
}: {
  className?: string
  onNavigate?: () => void
}) {
  const { t } = useTranslation('auth')
  const location = useLocation()
  const { data: admin } = useAdmin()
  const openCapture = useCaptureStore((state) => state.openCapture)
  const groups = getVisibleNavGroups({ isAdmin: admin?.admin === true })
  return (
    <aside className={cn('flex flex-col', className)}>
      <div className="px-2 pt-4">
        <Button
          className="w-full"
          onClick={() => {
            openCapture('upload')
            onNavigate?.()
          }}
        >
          <Plus className="mr-1 h-4 w-4" />
          {t('layout.newTranscription')}
        </Button>
      </div>
      <nav
        className="min-h-0 flex-1 space-y-4 overflow-y-auto px-2 py-6"
        aria-label={t('layout.sidebarNavigation')}
      >
        {groups.map((group) => (
          <div key={group.id} className="space-y-1">
            <p className="px-3 text-[0.65rem] font-medium uppercase tracking-wide text-muted-foreground">
              {t(group.labelKey)}
            </p>
            {group.routes.map((route) => (
              <NavLink
                key={route.path}
                route={route}
                active={location.pathname === route.path}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        ))}
      </nav>
    </aside>
  )
}

function NavLink({
  route,
  active,
  onNavigate,
}: {
  route: RouteConfig
  active: boolean
  onNavigate?: () => void
}) {
  const { t } = useTranslation('auth')
  const Icon = route.icon
  return (
    <Link
      to={route.path}
      target={route.openInNewTab ? '_blank' : undefined}
      rel={route.openInNewTab ? 'noreferrer' : undefined}
      onClick={onNavigate}
      className={cn(
        'flex items-center gap-3 rounded-md px-3 py-2 text-sm hover:bg-accent',
        active && 'bg-accent font-medium'
      )}
    >
      <Icon className="h-4 w-4" aria-hidden="true" />
      {route.labelKey ? t(route.labelKey) : route.label}
    </Link>
  )
}
