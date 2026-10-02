import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const devProxyTarget = process.env.VITE_DEV_PROXY_TARGET ?? 'http://localhost:8080'

// Dev-server proxy only. `changeOrigin` rewrites Host to the backend, so the
// browser's Origin (this dev server, e.g. http://127.0.0.1:5173 in the E2E
// harness) would look cross-origin to the backend's CORS allowlist and the MCP
// cross-origin check, and same-page API calls would be refused. The API
// authenticates with a bearer token, never cookies, so dropping Origin here
// opens no cross-site request path. Production builds do not use this proxy.
const stripBrowserOrigin = (proxy: {
  on: (event: string, callback: (request: { removeHeader: (name: string) => void }) => void) => void
}) => {
  proxy.on('proxyReq', (request) => request.removeHeader('origin'))
}

export default defineConfig({
  plugins: [react({ babel: { plugins: [['babel-plugin-react-compiler', {}]] } }), tailwindcss()],
  resolve: { alias: { '@': path.resolve(here, './src') } },
  server: {
    port: 5173,
    // Loopback only. Run `vite --host` to expose the dev server on a network.
    host: '127.0.0.1',
    proxy: {
      '/api/v1': { target: devProxyTarget, changeOrigin: true, configure: stripBrowserOrigin },
      '/mcp': { target: devProxyTarget, changeOrigin: true, configure: stripBrowserOrigin },
    },
  },
})
