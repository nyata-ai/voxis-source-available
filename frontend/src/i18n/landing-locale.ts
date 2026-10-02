/** Explicit public URLs outrank saved and detected language preferences. */
export function landingLocale(pathname: string): 'en' | 'id' | undefined {
  if (pathname === '/en' || pathname === '/en/') return 'en'
  if (pathname === '/id' || pathname === '/id/') return 'id'
  return undefined
}
