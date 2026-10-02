import {
  BookOpen,
  FileAudio,
  FileSearch,
  Home,
  Library,
  Mic,
  Settings,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react'

export type NavGroupId = 'work' | 'account'

export interface RouteConfig {
  path: string
  label: string
  icon: LucideIcon
  showInNav: boolean
  group?: NavGroupId
  labelKey?: string
  requiresAdmin?: boolean
  openInNewTab?: boolean
}

export const routes: Record<string, RouteConfig> = {
  dashboard: {
    path: '/dashboard',
    label: 'Dashboard',
    icon: Home,
    showInNav: true,
    group: 'work',
    labelKey: 'layout.nav.desk',
  },
  library: {
    path: '/library',
    label: 'Library',
    icon: Library,
    showInNav: true,
    group: 'work',
    labelKey: 'layout.nav.library',
  },
  media: { path: '/media', label: 'Audio Files', icon: FileAudio, showInNav: false },
  transcriptions: {
    path: '/transcriptions',
    label: 'Transcriptions',
    icon: FileSearch,
    showInNav: false,
  },
  summaries: { path: '/summaries', label: 'Summaries', icon: FileSearch, showInNav: false },
  record: {
    path: '/record',
    label: 'Record',
    icon: Mic,
    showInNav: true,
    group: 'work',
    labelKey: 'layout.nav.recording',
  },
  guide: {
    path: '/guide',
    label: 'User Guide',
    icon: BookOpen,
    showInNav: true,
    group: 'work',
    labelKey: 'layout.nav.guide',
    openInNewTab: true,
  },
  admin: {
    path: '/admin',
    label: 'Admin',
    icon: ShieldCheck,
    showInNav: true,
    group: 'account',
    labelKey: 'layout.nav.admin',
    requiresAdmin: true,
  },
  settings: {
    path: '/settings',
    label: 'Settings',
    icon: Settings,
    showInNav: true,
    group: 'account',
    labelKey: 'layout.nav.settings',
  },
}

export interface NavGroup {
  id: NavGroupId
  labelKey: string
  routes: RouteConfig[]
}

export function getVisibleNavGroups({ isAdmin }: { isAdmin: boolean }): NavGroup[] {
  const visible = Object.values(routes).filter(
    (route) => route.showInNav && (!route.requiresAdmin || isAdmin)
  )
  return [
    { id: 'work' as const, labelKey: 'layout.navGroup.work' },
    { id: 'account' as const, labelKey: 'layout.navGroup.account' },
  ]
    .map((group) => ({ ...group, routes: visible.filter((route) => route.group === group.id) }))
    .filter((group) => group.routes.length > 0)
}

export const CAPTURE_TABS = ['upload', 'record'] as const
export type CaptureTab = (typeof CAPTURE_TABS)[number]

export function isCaptureTab(value: string | null | undefined): value is CaptureTab {
  return CAPTURE_TABS.includes(value as CaptureTab)
}

export const captureHref = (tab: CaptureTab): string => `${routes.dashboard.path}?capture=${tab}`
