import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useActivity } from '@/hooks/useActivity'
import { cn } from '@/lib/utils'
import { ActivityCard } from './ActivityCard'
import { useRefreshListsOnSettle } from './useRefreshListsOnSettle'

const MAX_VISIBLE_CARDS = 4

export function ActivityTray() {
  const { t } = useTranslation('common')
  const { data } = useActivity()
  useRefreshListsOnSettle(data?.items)
  const [dismissed, setDismissed] = useState<Set<string>>(() => new Set())
  const visibleItems = useMemo(
    () => (data?.items ?? []).filter((item) => !dismissed.has(item.ref_id)),
    [data?.items, dismissed],
  )

  useEffect(() => {
    const refs = new Set((data?.items ?? []).map((item) => item.ref_id))
    setDismissed((current) => {
      const pruned = new Set([...current].filter((refID) => refs.has(refID)))
      return pruned.size === current.size ? current : pruned
    })
  }, [data?.items])

  const dismiss = useCallback((refID: string) => {
    setDismissed((current) => new Set(current).add(refID))
  }, [])

  if (visibleItems.length === 0) return null

  const cards = visibleItems.slice(0, MAX_VISIBLE_CARDS)
  const overflow = visibleItems.length - cards.length

  return (
    <aside
      aria-live="polite"
      className="fixed bottom-4 right-4 z-50 w-80 max-w-[calc(100vw-2rem)]"
    >
      <div className="space-y-2">
        {visibleItems.length > 1 && (
          <div className="flex items-center justify-between rounded-lg border border-background/20 bg-foreground px-3 py-2 text-sm text-background shadow-lg">
            <span className="font-medium">{t('activity.count', { count: visibleItems.length })}</span>
            {overflow > 0 && (
              <Button asChild variant="link" size="sm" className="h-7 px-0 text-background hover:text-background/80">
                <Link to="/library">{t('activity.more', { count: overflow })}</Link>
              </Button>
            )}
          </div>
        )}
        <div className={cn('space-y-2', visibleItems.length > 1 && 'max-h-[70vh] overflow-hidden')}>
          {cards.map((item) => (
            <ActivityCard key={item.ref_id} item={item} onDismiss={dismiss} />
          ))}
        </div>
      </div>
    </aside>
  )
}
