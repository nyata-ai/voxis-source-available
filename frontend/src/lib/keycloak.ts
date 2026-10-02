import Keycloak from 'keycloak-js'

export const keycloakConfig = {
  url:
    import.meta.env.VITE_KEYCLOAK_URL ||
    (typeof window === 'undefined' ? '/auth' : `${window.location.origin}/auth`),
  realm: import.meta.env.VITE_KEYCLOAK_REALM || 'voxis-oss',
  clientId: import.meta.env.VITE_KEYCLOAK_CLIENT_ID || 'voxis-oss-web',
}

export const supportsPkce =
  typeof window === 'undefined' ||
  ((window.isSecureContext || window.location.hostname === 'localhost') &&
    typeof window.crypto?.subtle !== 'undefined')
// Public routes: no session probe. Without a silent-SSO iframe the probe is a
// full-page redirect through Keycloak and back on every anonymous visit.
const anonymousPaths = ['/', '/en', '/en/', '/id', '/id/', '/terms', '/privacy', '/login', '/guide']

export function skipsSsoProbe(pathname: string): boolean {
  return anonymousPaths.includes(pathname)
}

export const initOptions: Keycloak.KeycloakInitOptions = {
  onLoad:
    typeof window !== 'undefined' && skipsSsoProbe(window.location.pathname)
      ? undefined
      : 'check-sso',
  checkLoginIframe: false,
  pkceMethod: 'S256',
  enableLogging: import.meta.env.DEV,
}

export const keycloak = new Keycloak(keycloakConfig)

function baseUrl(url: string): string {
  return url.endsWith('/') ? url.slice(0, -1) : url
}

export function getKeycloakAccountSecurityUrl(): string {
  return `${baseUrl(keycloakConfig.url)}/realms/${encodeURIComponent(keycloakConfig.realm)}/account/#/security/signingin`
}

export function getKeycloakChangePasswordUrl(): string {
  return getKeycloakAccountSecurityUrl()
}
export function getKeycloakManageMfaUrl(): string {
  return getKeycloakAccountSecurityUrl()
}

export function requestRecentLogin(): Promise<void> {
  return keycloak.login({ redirectUri: window.location.href, prompt: 'login', maxAge: 0 })
}

if (typeof window !== 'undefined' && import.meta.env.DEV) {
  ;(window as Window & { __keycloak?: typeof keycloak }).__keycloak = keycloak
}
