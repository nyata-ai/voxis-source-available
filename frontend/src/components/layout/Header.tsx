import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Menu, Mic, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { LanguageSwitcher } from './LanguageSwitcher'
import { UserButton } from './UserButton'
import { useCaptureStore } from '@/stores/capture'

export function Header({ onMenuClick }: { onMenuClick?: () => void }) {
  const { t } = useTranslation('auth')
  const openCapture = useCaptureStore((state) => state.openCapture)
  return (
    <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur">
      <div className="flex h-14 items-center gap-2 px-3 sm:px-4 lg:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          onClick={onMenuClick}
          aria-label={t('layout.toggleMenu')}
        >
          <Menu className="h-4 w-4" />
        </Button>
        <Link to="/dashboard" className="mr-2 text-sm font-semibold uppercase tracking-[0.18em]">
          Voxis
        </Link>
        <div className="hidden items-center gap-1 sm:flex">
          <Button variant="ghost" size="sm" onClick={() => openCapture('upload')}>
            <Upload className="mr-1 h-4 w-4" />
            {t('layout.quickAction.upload')}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => openCapture('record')}>
            <Mic className="mr-1 h-4 w-4" />
            {t('layout.quickAction.record')}
          </Button>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <LanguageSwitcher />
          <nav aria-label={t('layout.mainNavigation')}>
            <UserButton />
          </nav>
        </div>
      </div>
    </header>
  )
}
