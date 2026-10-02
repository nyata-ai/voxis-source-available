/**
 * A stand-in for the `keycloak-js` default export, aliased in by
 * `vite.capture.config.ts` so the guide screenshot harness can render
 * authenticated routes with no identity provider anywhere in the picture.
 *
 * It implements exactly the surface `src/lib/keycloak.ts` and
 * `src/contexts/AuthContext.tsx` touch: construct, `init()`, the parsed tokens,
 * `updateToken()`, `login()`/`logout()` and the four event hooks. Nothing here
 * signs anything — the token is a three-part unsigned JWT whose only job is to
 * ride along in an `Authorization` header that a mocked API never inspects.
 */

/** The invented reader every capture is signed in as. No e-mail address: the
 *  guide is public content, and the header renders this verbatim. */
const DEMO_NAME = 'Demo Reviewer'
const DEMO_SECONDARY = 'Demo workspace'
const DEMO_SUBJECT = '00000000-0000-4000-8000-000000000001'

/** Far enough out that no capture run can trip the refresh path. */
const EXPIRY_SECONDS = 4_102_444_800 // 2100-01-01T00:00:00Z

function base64url(value: string): string {
  return btoa(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export interface FakeTokenParsed {
  exp: number
  iat: number
  sub: string
  aud: string
  iss: string
  typ: string
  azp: string
  name: string
  preferred_username: string
  email: string
  email_verified: boolean
}

function claims(): FakeTokenParsed {
  return {
    exp: EXPIRY_SECONDS,
    iat: 1_700_000_000,
    sub: DEMO_SUBJECT,
    aud: 'voxis-api',
    iss: 'https://auth.invalid/auth/realms/voxis-oss',
    typ: 'Bearer',
    azp: 'voxis-oss-web',
    name: DEMO_NAME,
    preferred_username: 'demo',
    // AuthContext refuses a token without this claim, and the header renders it
    // under the display name — so it carries a label, never an address.
    email: DEMO_SECONDARY,
    email_verified: true,
  }
}

function encodeToken(payload: FakeTokenParsed): string {
  const header = base64url(JSON.stringify({ alg: 'none', typ: 'JWT' }))
  const body = base64url(JSON.stringify(payload))
  return `${header}.${body}.`
}

/** Matches the subset of `Keycloak.KeycloakInstance` the app consumes. */
export default class FakeKeycloak {
  authenticated = true
  token: string
  refreshToken: string
  idToken: string
  tokenParsed: FakeTokenParsed
  idTokenParsed: FakeTokenParsed
  subject = DEMO_SUBJECT
  realmAccess = { roles: [] as string[] }
  resourceAccess = {} as Record<string, { roles: string[] }>

  onReady?: (authenticated?: boolean) => void
  onAuthSuccess?: () => void
  onAuthError?: (error: unknown) => void
  onTokenExpired?: () => void

  constructor(_config?: unknown) {
    const payload = claims()
    this.tokenParsed = payload
    this.idTokenParsed = payload
    this.token = encodeToken(payload)
    this.idToken = this.token
    this.refreshToken = this.token
  }

  async init(_options?: unknown): Promise<boolean> {
    this.onReady?.(true)
    this.onAuthSuccess?.()
    return true
  }

  /** False means "still valid, nothing refreshed" — the app's happy path. */
  async updateToken(_minValidity?: number): Promise<boolean> {
    return false
  }

  async login(_options?: unknown): Promise<void> {}

  async logout(_options?: unknown): Promise<void> {}

  createLoginUrl(_options?: unknown): string {
    return '/'
  }

  createLogoutUrl(_options?: unknown): string {
    return '/'
  }

  isTokenExpired(_minValidity?: number): boolean {
    return false
  }

  clearToken(): void {}
}
