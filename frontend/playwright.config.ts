import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.VOXIS_OSS_E2E_BASE_URL ?? 'http://127.0.0.1:5173'
const startCommand = process.env.VOXIS_OSS_E2E_START_COMMAND
const browserRecording = process.env.VOXIS_OSS_RECORDING_E2E === '1'

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: 'list',
  timeout: 60_000,
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    permissions: browserRecording ? ['microphone'] : [],
    launchOptions: browserRecording
      ? { args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'] }
      : undefined,
    timezoneId: 'UTC',
    locale: 'en-US',
  },
  webServer: startCommand ? {
    command: startCommand,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  } : undefined,
  projects: [
    { name: 'public', testMatch: /.*\.public\.spec\.ts/ },
    { name: 'setup', testMatch: /.*\.setup\.ts/ },
    {
      name: 'chromium',
      testIgnore: [/.*\.setup\.ts/, /.*\.public\.spec\.ts/],
      use: { ...devices['Desktop Chrome'], storageState: './tests/e2e/.auth/user.json' },
      dependencies: ['setup'],
    },
  ],
})
