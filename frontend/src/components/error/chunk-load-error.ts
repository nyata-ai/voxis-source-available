/*
 * Detection + reload guard for failed lazy-route imports, shared by
 * RouteErrorRecovery. Lives outside the component file so the component
 * module stays fast-refresh clean.
 */

export const RELOAD_GUARD_KEY = 'voxis.chunk-reload-at'
const RELOAD_GUARD_WINDOW_MS = 30_000

// Firefox: "error loading dynamically imported module: <url>"
// Chrome:  "Failed to fetch dynamically imported module: <url>"
// Safari:  "Importing a module script failed."
const CHUNK_LOAD_PATTERN =
  /dynamically imported module|Importing a module script failed/i

export function isChunkLoadError(error: unknown): boolean {
  return error instanceof Error && CHUNK_LOAD_PATTERN.test(error.message)
}

// Records the reload attempt BEFORE the reload happens, so a chunk that keeps
// failing can never loop the page. Returns false when a recent attempt exists
// or when storage is unusable — with no way to record the guard, reloading
// would risk an unbounded loop, so the fallback UI is the safer path.
export function armReloadGuard(): boolean {
  try {
    const previous = Number(sessionStorage.getItem(RELOAD_GUARD_KEY) ?? 0)
    if (Date.now() - previous < RELOAD_GUARD_WINDOW_MS) return false
    sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now()))
    return true
  } catch {
    return false
  }
}
