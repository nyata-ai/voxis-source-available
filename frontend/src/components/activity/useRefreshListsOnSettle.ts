import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { mediaKeys } from '@/hooks/useMedia'
import { transcriptionKeys } from '@/hooks/useTranscription'
import type { ActivityItem, ActivityStatus } from '@/types/activity'

// Refresh cached Library data when the activity poll reports that work has
// settled. The first result only seeds the comparison.
export function useRefreshListsOnSettle(items: ActivityItem[] | undefined) {
  const queryClient = useQueryClient()
  const previous = useRef<Map<string, ActivityStatus> | null>(null)

  useEffect(() => {
    if (!items) return
    const before = previous.current
    const next = new Map(items.map((item) => [item.ref_id, item.status] as const))
    previous.current = next
    if (before === null) return

    let settled = false
    for (const [refID, status] of next) {
      const previousStatus = before.get(refID)
      if (previousStatus !== undefined && previousStatus !== status && status !== 'in_progress') settled = true
    }
    for (const refID of before.keys()) {
      if (!next.has(refID)) settled = true
    }
    if (!settled) return

    void queryClient.invalidateQueries({ queryKey: mediaKeys.all })
    void queryClient.invalidateQueries({ queryKey: transcriptionKeys.all })
  }, [items, queryClient])
}
