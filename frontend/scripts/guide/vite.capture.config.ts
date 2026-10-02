import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, mergeConfig, type ConfigEnv, type UserConfig } from 'vite'
import baseConfig from '../../vite.config'

const here = path.dirname(fileURLToPath(import.meta.url))
export const CAPTURE_PORT = 5199
process.env.VITE_API_URL = '/api/v1'
delete process.env.VITE_UPLOAD_URL

type ConfigFn = (env: ConfigEnv) => UserConfig | Promise<UserConfig>

export default defineConfig(async (env) => {
  const base = typeof baseConfig === 'function' ? await (baseConfig as ConfigFn)(env) : baseConfig
  const merged = mergeConfig(base, { resolve: { alias: { 'keycloak-js': path.resolve(here, 'fake-keycloak.ts') } } })
  merged.server = { port: CAPTURE_PORT, strictPort: true, host: '127.0.0.1' }
  return merged
})
