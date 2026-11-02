import { useSyncExternalStore } from 'react'
import type { ScrollMode } from '@/hooks/useTranscriptAutoScroll'

/** Tailwind's `lg`: where the reader becomes two columns and the transcript
 *  gets its own scroll box. Keep in step with the `lg:` classes that cap it. */
const WIDE_QUERY = '(min-width: 1024px)'

function subscribe(onChange: () => void): () => void {
  if (typeof window.matchMedia !== 'function') return () => {}
  const query = window.matchMedia(WIDE_QUERY)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

/** Without `matchMedia` (jsdom, very old browsers) the reader keeps the
 *  desktop behaviour it always had: the box scrolls. */
function snapshot(): ScrollMode {
  if (typeof window.matchMedia !== 'function') return 'container'
  return window.matchMedia(WIDE_QUERY).matches ? 'container' : 'window'
}

/** Which element follow-playback scrolls: the transcript box from `lg` up,
 *  the window below it, where the page is the only scroll. */
export function useReaderScrollMode(): ScrollMode {
  return useSyncExternalStore(subscribe, snapshot, () => 'container')
}
