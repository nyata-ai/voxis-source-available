import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

// Set before the config is evaluated so worker processes inherit it. Node reads
// TZ when it first resolves a timezone, and tests/setup.ts imports the real i18n
// (which now runs geo detection), so an unpinned host silently changes the UI
// language every assertion depends on.
process.env.TZ = 'UTC'

export default defineConfig({
  // Mirrors vite.config.ts: tests must exercise the compiled components the
  // production bundle ships, or a Rules-of-React violation the compiler turns
  // into a behaviour change would pass here and fail in the browser.
  plugins: [react({ babel: { plugins: [['babel-plugin-react-compiler', {}]] } })],
  test: {
    globals: true,
    environment: 'jsdom',
    env: { TZ: 'UTC' },
    setupFiles: ['./tests/setup.ts'],
    // Vitest's 5 s default is a machine-speed assumption, not a correctness one.
    // Under full parallelism (CI runners, a laptop running the backend suite at
    // the same time) render-heavy tests crossed it and failed spuriously. A test
    // that genuinely hangs still fails — three seconds later.
    testTimeout: 15_000,
    include: ['src/**/*.{test,spec}.{ts,tsx}', 'tests/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['tests/e2e/**'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: [
        'node_modules/',
        'tests/',
        '**/*.d.ts',
        '**/*.config.*',
        'src/main.tsx',
        'src/components/ui/**', // shadcn/ui components - generated code
      ],
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
})
