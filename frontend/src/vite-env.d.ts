/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL: string
  readonly VITE_KEYCLOAK_URL: string
  readonly VITE_KEYCLOAK_REALM: string
  readonly VITE_KEYCLOAK_CLIENT_ID: string
  readonly VITE_RECORDING_MAX_DURATION_MS?: string
  readonly VITE_RECORDING_HEARTBEAT_INTERVAL_MS?: string
  /** Origin serving the two upload routes. Unset ⇒ same origin as the API. */
  readonly VITE_UPLOAD_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
