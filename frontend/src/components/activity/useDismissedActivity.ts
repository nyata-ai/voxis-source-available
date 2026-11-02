import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ActivityItem } from '@/types/activity'

const STORAGE_KEY = 'voxis-activity-dismissed'
/** The feed keeps failures for 48 hours; nobody dismisses more than this in one
 *  session, and the list must not grow without limit if a write ever goes wrong. */
export const MAX_REMEMBERED = 50

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string')
}

function readRemembered(): ReadonlySet<string> {
  try {
    const parsed: unknown = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? '[]')
    return new Set(isStringArray(parsed) ? parsed.slice(-MAX_REMEMBERED) : [])
  } catch {
    return new Set()
  }
}

function writeRemembered(refs: ReadonlySet<string>): void {
  try {
    if (refs.size === 0) window.sessionStorage.removeItem(STORAGE_KEY)
    else window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify([...refs].slice(-MAX_REMEMBERED)))
  } catch {
    // Blocked or full storage: the dismissal still holds until the next reload.
  }
}

/** The same set when nothing is dropped, so React skips the re-render. */
function keepOnly(current: ReadonlySet<string>, keep: ReadonlySet<string>): ReadonlySet<string> {
  const kept = new Set([...current].filter((ref) => keep.has(ref)))
  return kept.size === current.size ? current : kept
}

function withRef(current: ReadonlySet<string>, ref: string): ReadonlySet<string> {
  return new Set(current).add(ref)
}

/**
 * Which activity items the tray still shows.
 *
 * A dismissed failure is remembered for the session, because the feed keeps
 * failures for two days and would otherwise bring the card back on every reload.
 * Any other dismissal lasts for the page's life: a finished card is about to
 * leave the feed by itself. Only a failure the reader actually saw counts: a
 * running job dismissed earlier that then fails comes back, and a retried job
 * that fails again comes back, so a failure cannot vanish unseen on a phone.
 */
export function useDismissedActivity(items: ActivityItem[] | undefined) {
  const [dismissed, setDismissed] = useState<ReadonlySet<string>>(() => new Set())
  const [failures, setFailures] = useState<ReadonlySet<string>>(readRemembered)
  const itemsRef = useRef(items)

  useEffect(() => {
    itemsRef.current = items
    // Before the first read there is nothing to compare with, and a reload must
    // not forget what it just restored.
    if (!items) return
    const inFeed = new Set(items.map((item) => item.ref_id))
    const failed = new Set(items.filter((item) => item.status === 'failed').map((item) => item.ref_id))
    setDismissed((current) => keepOnly(current, inFeed))
    setFailures((current) => keepOnly(current, failed))
  }, [items])

  useEffect(() => writeRemembered(failures), [failures])

  // Stable on purpose: a card's auto-dismiss timer restarts when this changes.
  const dismiss = useCallback((refID: string) => {
    const item = itemsRef.current?.find((candidate) => candidate.ref_id === refID)
    if (item?.status === 'failed') setFailures((current) => withRef(current, refID))
    else setDismissed((current) => withRef(current, refID))
  }, [])

  const visibleItems = useMemo(
    () =>
      (items ?? []).filter(
        (item) => !(item.status === 'failed' ? failures : dismissed).has(item.ref_id),
      ),
    [items, dismissed, failures],
  )

  return { visibleItems, dismiss }
}
