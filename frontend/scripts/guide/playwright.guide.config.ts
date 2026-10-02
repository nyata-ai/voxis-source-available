/**
 * Playwright config for the guide screenshot harness. Separate from the app's
 * `playwright.config.ts` on purpose: that one drives the real dev server and a
 * real Keycloak, this one drives an offline capture server and writes images.
 *
 * Run it with `npm run guide:screenshots`.
 */
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'
import { CAPTURE_PORT } from './vite.capture.config'

const baseURL = `http://127.0.0.1:${CAPTURE_PORT}`
/** Playwright resolves `webServer.cwd` from this file's directory by default;
 *  the Vite command has to run from the frontend root. */
const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')

export default defineConfig({
  testDir: '.',
  testMatch: '**/*.spec.ts',
  // Captures share one dev server and write into one directory; a retry would
  // silently overwrite a good image with a worse one.
  retries: 0,
  workers: 1,
  timeout: 120_000,
  reporter: [['list']],
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    // The guide's capture settings: 1280x800, light, English, no HiDPI.
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    colorScheme: 'light',
    locale: 'en-US',
    timezoneId: 'UTC',
    // Nothing here talks to a network, so a stuck request is a bug, not a wait.
    actionTimeout: 15_000,
  },
  webServer: {
    command: 'npx vite --config scripts/guide/vite.capture.config.ts',
    cwd: frontendRoot,
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})
