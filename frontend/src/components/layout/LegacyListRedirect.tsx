import { Navigate, useSearchParams } from 'react-router-dom'

/** Pre-Library `?source=` values, mapped onto the Library's filter
 * vocabulary. Anything else passes through untouched — LibraryPage already
 * falls back to "all" for a value it doesn't recognise. */
const LEGACY_SOURCE_VALUES: Record<string, string> = {
  media: 'audio',
  url: 'links',
}

/** Translates a legacy list route's query string into the Library's. */
function mapLegacyParams(params: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams()

  for (const [key, value] of params) {
    // `/media?upload=1` opened MediaLibraryPage's own upload modal. The
    // capture modal owns uploads now, and MainLayout opens it from `?capture=`
    // on any authenticated route.
    if (key === 'upload') {
      if (value === '1') next.set('capture', 'upload')
      continue
    }

    if (key === 'source') {
      next.set('source', LEGACY_SOURCE_VALUES[value] ?? value)
      continue
    }

    next.append(key, value)
  }

  return next
}

interface LegacyListRedirectProps {
  /** Path to land on, without a query string. */
  to: string
}

/**
 * One redirect for every list route the Library replaced (/media,
 * /transcriptions, /summaries, /upload). Bookmarks and in-app links that
 * still carry a legacy query param keep working: the param is translated,
 * not dropped.
 */
export function LegacyListRedirect({ to }: LegacyListRedirectProps) {
  const [searchParams] = useSearchParams()
  const query = mapLegacyParams(searchParams).toString()

  return <Navigate to={query ? `${to}?${query}` : to} replace />
}
