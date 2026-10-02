import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { LogOut, Monitor, Moon, Settings, Sun } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuth } from '@/contexts/AuthContext'
import { useThemeStore, type Theme } from '@/stores/theme'
import { useUpdatePreferences } from '@/hooks/useSettings'
import { countAllUnsent } from '@/lib/chunk-outbox'
import { SignOutWarningDialog } from './SignOutWarningDialog'

function firstGrapheme(value: string | undefined): string | null {
  if (!value) return null
  // Array.from handles astral-plane characters (emoji, some CJK) correctly,
  // whereas `value[0]` returns a lone UTF-16 surrogate half.
  const first = Array.from(value)[0]
  return first?.toUpperCase() ?? null
}

export function UserButton() {
  const { t } = useTranslation('auth')
  const { user, logout } = useAuth()
  const theme = useThemeStore((s) => s.theme)
  const setTheme = useThemeStore((s) => s.setTheme)
  const { mutate: updatePreferences } = useUpdatePreferences()
  const [confirmSignOut, setConfirmSignOut] = useState(false)

  if (!user) return null

  function handleThemeChange(next: Theme) {
    setTheme(next)
    // Mirror AppearanceSection — persist server-side so the choice survives device changes.
    updatePreferences({ theme: next })
  }

  // Sign-out wipes the browser's unsent recording audio; warn before that
  // loses anything. If the store cannot be read, sign out anyway.
  async function handleSignOut() {
    let unsent = 0
    try {
      unsent = await countAllUnsent()
    } catch {
      unsent = 0
    }
    if (unsent > 0) {
      setConfirmSignOut(true)
      return
    }
    void logout()
  }

  const initial = firstGrapheme(user.name) ?? firstGrapheme(user.email) ?? '?'
  const displayName = user.name || user.username || user.email

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="flex items-center gap-2 border border-transparent px-1.5 py-1 transition-colors hover:border-border hover:bg-accent/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            aria-label={t('layout.accountMenu', { name: displayName })}
          >
            <span className="flex h-7 w-7 items-center justify-center bg-primary text-[0.7rem] font-semibold text-primary-foreground">
              {initial}
            </span>
            <span className="hidden text-left leading-tight md:block">
              <span className="block text-xs font-semibold">{displayName}</span>
              <span className="block text-[0.68rem] text-muted-foreground">{user.email}</span>
            </span>
          </button>
        </DropdownMenuTrigger>

        <DropdownMenuContent align="end" sideOffset={8} className="w-56">
          <DropdownMenuLabel className="font-normal">
            <div className="space-y-0.5">
              <p className="truncate text-sm font-medium leading-none">{displayName}</p>
              <p className="truncate text-xs text-muted-foreground">{user.email}</p>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />

          <DropdownMenuItem asChild>
            <Link to="/settings" className="flex w-full items-center">
              <Settings className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
              {t('layout.settings')}
            </Link>
          </DropdownMenuItem>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <ThemeIcon theme={theme} />
              <span className="ml-2">{t('layout.theme')}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuRadioGroup
                value={theme}
                onValueChange={(v) => handleThemeChange(v as Theme)}
              >
                <DropdownMenuRadioItem value="light">
                  <Sun className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                  {t('layout.light')}
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="dark">
                  <Moon className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                  {t('layout.dark')}
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="system">
                  <Monitor className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                  {t('layout.system')}
                </DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSeparator />

          <DropdownMenuItem onClick={() => void handleSignOut()} className="text-foreground">
            <LogOut className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
            {t('layout.signOut')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <SignOutWarningDialog
        open={confirmSignOut}
        onOpenChange={setConfirmSignOut}
        onConfirm={() => void logout()}
      />
    </>
  )
}

function ThemeIcon({ theme }: { theme: Theme }) {
  if (theme === 'light') return <Sun className="h-3.5 w-3.5" aria-hidden="true" />
  if (theme === 'dark') return <Moon className="h-3.5 w-3.5" aria-hidden="true" />
  return <Monitor className="h-3.5 w-3.5" aria-hidden="true" />
}
