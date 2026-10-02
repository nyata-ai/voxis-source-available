import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('keycloak-js', () => ({
  default: class MockKeycloak {
    init = vi.fn()
    login = vi.fn()
    logout = vi.fn()
    updateToken = vi.fn()
  },
}))

describe('Voxis-OSS Keycloak defaults', () => {
  beforeEach(() => {
    vi.resetModules()
    window.history.replaceState({}, '', '/login')
  })

  it('uses the bundled realm, web client, same-origin auth path, and S256', async () => {
    const { initOptions, keycloakConfig } = await import('./keycloak')

    expect(keycloakConfig.realm).toBe('voxis-oss')
    expect(keycloakConfig.clientId).toBe('voxis-oss-web')
    expect(keycloakConfig.url).toBe(window.location.origin + '/auth')
    expect(initOptions.pkceMethod).toBe('S256')
    expect(initOptions.checkLoginIframe).toBe(false)
  })

  it('keeps landing, legal, login, and guide routes outside the session probe', async () => {
    const { skipsSsoProbe } = await import('./keycloak')

    for (const path of ['/', '/en', '/id', '/terms', '/privacy', '/login', '/guide']) {
      expect(skipsSsoProbe(path)).toBe(true)
    }
    expect(skipsSsoProbe('/dashboard')).toBe(false)
  })

  it('builds the account-security page in the bundled realm', async () => {
    const { getKeycloakAccountSecurityUrl } = await import('./keycloak')

    expect(getKeycloakAccountSecurityUrl()).toBe(
      window.location.origin + '/auth/realms/voxis-oss/account/#/security/signingin'
    )
  })
})
