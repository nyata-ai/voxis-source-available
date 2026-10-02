import { Buffer } from 'node:buffer'
import { createHmac } from 'node:crypto'
import { test as setup, expect, type Page } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

const AUTH_FILE = './tests/e2e/.auth/user.json'
const TOTP_PERIOD_MS = 30_000
type RequiredSetting =
  | 'VOXIS_OSS_E2E_USERNAME'
  | 'VOXIS_OSS_E2E_PASSWORD'
  | 'VOXIS_OSS_E2E_ISSUER'

function configured(name: RequiredSetting): string {
  const value = process.env[name]
  if (!value) throw new Error(`${name} must be set for bundled-Keycloak browser tests.`)
  return value
}

function configuredTotpSecret(): string {
  const value = process.env.VOXIS_OSS_E2E_TOTP_SECRET
  if (!value) {
    throw new Error('VOXIS_OSS_E2E_TOTP_SECRET is required when Keycloak requests an OTP.')
  }
  return value
}

function base32Secret(secret: string): Buffer {
  const normalized = secret.replace(/[\s=-]/g, '').toUpperCase()
  if (!/^[A-Z2-7]+$/.test(normalized)) {
    throw new Error('VOXIS_OSS_E2E_TOTP_SECRET must be an unpadded Base32 value.')
  }

  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const bytes: number[] = []
  let value = 0
  let bits = 0
  for (const character of normalized) {
    value = (value << 5) | alphabet.indexOf(character)
    bits += 5
    if (bits < 8) continue
    bits -= 8
    bytes.push((value >>> bits) & 0xff)
  }
  return Buffer.from(bytes)
}

function currentTotp(secret: string, now = Date.now()): string {
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(now / TOTP_PERIOD_MS)))
  const digest = createHmac('sha1', base32Secret(secret)).update(counter).digest()
  const offset = digest[digest.length - 1] & 0x0f
  const code =
    (((digest[offset] & 0x7f) << 24) |
      (digest[offset + 1] << 16) |
      (digest[offset + 2] << 8) |
      digest[offset + 3]) >>>
    0
  return String(code % 1_000_000).padStart(6, '0')
}

async function completeLogin(page: Page, appOrigin: string) {
  const reachesApp = page.waitForURL(
    (url) => url.origin === appOrigin && url.pathname === '/dashboard',
    { timeout: 15_000 }
  )
  const otpField = page.locator('#otp')
  const nextStep = await Promise.race([
    reachesApp.then(() => 'app' as const),
    otpField.waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'otp' as const),
  ])
  if (nextStep === 'app') return

  await otpField.fill(currentTotp(configuredTotpSecret()))
  await page.click('#kc-login')
  await reachesApp
}

setup('authenticate with the bundled Voxis-OSS Keycloak realm', async ({ page, baseURL }) => {
  const username = configured('VOXIS_OSS_E2E_USERNAME')
  const password = configured('VOXIS_OSS_E2E_PASSWORD')
  const issuer = configured('VOXIS_OSS_E2E_ISSUER').replace(/\/$/, '')
  const appOrigin = new URL(baseURL ?? 'http://127.0.0.1:5173').origin
  if (!issuer.endsWith('/realms/voxis-oss')) {
    throw new Error('VOXIS_OSS_E2E_ISSUER must identify the voxis-oss realm.')
  }

  await page.goto('/dashboard')
  await page.waitForURL((url) => url.href.startsWith(issuer), { timeout: 15_000 })
  await page.fill('#username', username)
  await page.fill('#password', password)
  await page.click('#kc-login')
  await completeLogin(page, appOrigin)
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible({ timeout: 10_000 })

  await page.context().storageState({ path: AUTH_FILE })
})
