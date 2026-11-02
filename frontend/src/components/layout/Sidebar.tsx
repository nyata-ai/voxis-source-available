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
          <div key={group.id} className="space-y-px">
            <p className="kicker px-3 pb-1">
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
        // 44px rows in the phone drawer (touch-target rule); about 36px from md.
        'group flex min-h-11 items-center gap-2.5 border-l-2 px-3 py-2 text-body transition-colors md:min-h-0',
        active
          ? 'border-l-coral-mark bg-accent/40 text-foreground'
          : 'border-l-transparent text-muted-foreground hover:bg-accent/25 hover:text-foreground'
      )}
    >
      <Icon
        className={cn(
          'h-3.5 w-3.5 transition-opacity',
          active ? 'opacity-100' : 'opacity-70 group-hover:opacity-100'
        )}
        aria-hidden="true"
      />
      {route.labelKey ? t(route.labelKey) : route.label}
    </Link>
  )
}
