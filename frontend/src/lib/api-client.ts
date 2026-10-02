/* Voxis Source-Available API client. See the repository licensing files. */

import { keycloak } from './keycloak'
import { useAccessStore } from '@/stores/access'
import type { MediaStreamUrlResponse } from '@/types/media'

// Bundled deployments use the reverse proxy on this same origin. Local tooling
// can override it with VITE_API_URL.
export const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1'

/**
 * Origin that terminates the two body-heavy upload requests, when it differs
 * from the API origin.
 *
 * A deployment may sit behind a CDN or reverse proxy that caps request bodies
 * well under the upload limit. Such a deployment can point these two requests
 * at a separate upload host that bypasses the proxy. Unset (the default) means
 * "same origin as the API", so no other deployment needs any change.
 *
 * Consequence to keep in mind when touching this: once the origins differ, the
 * request is cross-origin and carries an Authorization header, so the browser
 * preflights it. The backend already answers OPTIONS; nginx on the upload host
 * must route OPTIONS as well as POST.
 *
 * Not supported in Voxis Source-Available: the bundled Caddy proxy sends
 * `connect-src 'self'` in its Content Security Policy, so the browser blocks
 * any request to a separate upload origin. Leave VITE_UPLOAD_URL unset; the
 * bundled proxy has no request-size cap below the upload limit.
 */
const UPLOAD_ORIGIN = (import.meta.env.VITE_UPLOAD_URL ?? '').trim()

/**
 * Endpoints served by the upload origin — exactly the two the upload host's
 * nginx block routes. Anything else stays on the API origin, so a future caller
 * of apiClient.upload() cannot silently aim at a host that would 404 it.
 */
const UPLOAD_ROUTES: readonly RegExp[] = [/^\/media\/upload$/, /^\/recordings\/[^/?#]+\/chunks$/]

/** True when `endpoint` is one of the two upload-origin routes. */
export function isUploadRoute(endpoint: string): boolean {
  const path = endpoint.split(/[?#]/)[0]
  return UPLOAD_ROUTES.some((pattern) => pattern.test(path))
}

/**
 * Path prefix of the API base (`/api/v1`), used to rebuild the same path on the
 * upload origin. Falls back to treating the base as a bare path, which is what
 * a same-origin `VITE_API_URL=/api/v1` build would set.
 */
function apiPathPrefix(apiBaseUrl: string): string {
  try {
    const { pathname } = new URL(apiBaseUrl)
    return pathname === '/' ? '' : pathname.replace(/\/+$/, '')
  } catch {
    return apiBaseUrl.replace(/\/+$/, '')
  }
}

/**
 * Base URL for one request: the upload origin for the two upload routes when
 * VITE_UPLOAD_URL is set, the API base otherwise.
 *
 * Only the ORIGIN of VITE_UPLOAD_URL is used; the path prefix always comes from
 * the API base. That way `https://upload.example.com` and
 * `https://upload.example.com/api/v1` both produce the same correct URL — an
 * operator writing the plain origin (which is how the deployment plan describes
 * it) cannot ship a build that posts to a path the upload host 404s.
 *
 * The arguments exist so tests can exercise set/unset without a rebuild.
 */
export function resolveRequestBaseUrl(
  endpoint: string,
  uploadOrigin: string = UPLOAD_ORIGIN,
  apiBaseUrl: string = API_BASE_URL
): string {
  const configured = uploadOrigin.trim()
  if (!configured || !isUploadRoute(endpoint)) return apiBaseUrl

  let origin: string
  try {
    origin = new URL(configured).origin
  } catch {
    // Build-time misconfiguration. Failing loudly beats silently sending a large
    // body through the proxied origin, where it can die at the proxy's body cap
    // with an error page that names nothing.
    throw new Error(`VITE_UPLOAD_URL is not a valid absolute URL: ${configured}`)
  }
  return `${origin}${apiPathPrefix(apiBaseUrl)}`
}

export function resolveApiUrl(url: string): string {
  if (!url) return url
  try {
    return new URL(url, API_BASE_URL).toString()
  } catch {
    return url
  }
}

// Request timeout in milliseconds
const REQUEST_TIMEOUT_MS = 30_000

// Upload timeout in milliseconds — large files need time for server-side
// encryption and storage after bytes finish sending to the reverse proxy.
const UPLOAD_TIMEOUT_MS = 600_000

// Token refresh settings
const TOKEN_MIN_VALIDITY_SECONDS = 30

export class ApiError extends Error {
  /** Seconds from a Retry-After header, when the server sent one. */
  public retryAfter?: number

  constructor(
    public status: number,
    message: string,
    public data?: unknown
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * True for the server's "signed in, but this account lacks the app role"
 * answer: 403 with `{"error": "role_required"}`. Not a session problem, so it
 * must never trigger a login redirect (that would loop through Keycloak).
 */
export function isRoleRequiredError(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 403) return false
  const data = error.data
  return (
    typeof data === 'object' &&
    data !== null &&
    (data as { error?: unknown }).error === 'role_required'
  )
}

/** Surface a missing app role once, app-wide, instead of per failing page. */
function noteAccessDenial(error: ApiError): void {
  if (isRoleRequiredError(error)) {
    useAccessStore.getState().markRoleRequired()
  }
}

/** Parses a delay-seconds Retry-After header; HTTP-date forms are ignored. */
function parseRetryAfterSeconds(value: string | null): number | undefined {
  if (!value) return undefined
  const seconds = Number(value)
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : undefined
}

// Mutex to prevent concurrent token refresh attempts
let refreshPromise: Promise<boolean> | null = null

async function ensureValidToken(): Promise<boolean> {
  // If token is undefined, we can't make authenticated requests
  if (!keycloak.token) {
    return false
  }

  // If already refreshing, wait for that to complete
  if (refreshPromise) {
    return refreshPromise
  }

  try {
    refreshPromise = keycloak.updateToken(TOKEN_MIN_VALIDITY_SECONDS)
    return await refreshPromise
  } catch {
    // Token refresh failed
    return false
  } finally {
    refreshPromise = null
  }
}

interface InternalRequestOptions extends RequestInit {
  timeout?: number
  skipContentType?: boolean
  /** Origin + path prefix to prepend. Defaults to the API base. */
  baseUrl?: string
}

async function _baseFetch(
  endpoint: string,
  options: InternalRequestOptions = {},
  headers: Record<string, string> = {}
): Promise<Response> {
  const {
    timeout = REQUEST_TIMEOUT_MS,
    skipContentType,
    baseUrl = API_BASE_URL,
    ...fetchOptions
  } = options

  const url = `${baseUrl}${endpoint}`

  // Set up request timeout
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeout)

  const requestHeaders: Record<string, string> = { ...headers }
  if (!skipContentType) {
    requestHeaders['Content-Type'] = 'application/json'
  }

  try {
    const response = await fetch(url, {
      ...fetchOptions,
      signal: controller.signal,
      headers: requestHeaders,
    })

    clearTimeout(timeoutId)

    if (!response.ok) {
      let errorData: unknown
      try {
        const text = await response.text()
        errorData = text ? JSON.parse(text) : null
      } catch {
        errorData = null
      }

      const apiError = new ApiError(response.status, response.statusText, errorData)
      apiError.retryAfter = parseRetryAfterSeconds(response.headers.get('Retry-After'))
      noteAccessDenial(apiError)
      throw apiError
    }

    return response
  } catch (err) {
    clearTimeout(timeoutId)

    // Handle timeout
    if (err instanceof Error && err.name === 'AbortError') {
      throw new ApiError(408, 'Request timeout')
    }

    throw err
  }
}

async function _authenticatedFetch(
  endpoint: string,
  options: InternalRequestOptions = {}
): Promise<Response> {
  // Ensure we have a valid token before making the request
  const tokenValid = await ensureValidToken()

  if (!tokenValid && !keycloak.token) {
    // No valid token and refresh failed - redirect to login
    if (import.meta.env.DEV) {
      console.error('No valid token available, redirecting to login')
    }
    keycloak.login()
    throw new ApiError(401, 'Authentication required')
  }

  const headers: Record<string, string> = {
    Authorization: `Bearer ${keycloak.token}`,
  }

  try {
    return await _baseFetch(endpoint, options, headers)
  } catch (err) {
    // Handle 401 by redirecting to login
    if (err instanceof ApiError && err.status === 401) {
      if (import.meta.env.DEV) {
        console.error('Session expired, redirecting to login')
      }
      keycloak.login()
      throw new ApiError(401, 'Session expired', err.data)
    }
    throw err
  }
}

async function request<T>(endpoint: string, options: InternalRequestOptions = {}): Promise<T> {
  const response = await _authenticatedFetch(endpoint, options)

  // Handle empty responses (204 No Content, etc.)
  if (response.status === 204 || response.headers.get('content-length') === '0') {
    return null as T
  }

  const text = await response.text()
  return text ? JSON.parse(text) : null
}

async function publicRequest<T>(
  endpoint: string,
  options: InternalRequestOptions = {}
): Promise<T> {
  const response = await _baseFetch(endpoint, options)

  // Handle empty responses (204 No Content, etc.)
  if (response.status === 204 || response.headers.get('content-length') === '0') {
    return null as T
  }

  const text = await response.text()
  return text ? JSON.parse(text) : null
}

export const publicApiClient = {
  get: <T>(endpoint: string) => publicRequest<T>(endpoint, { method: 'GET' }),

  post: <T>(endpoint: string, data?: unknown) =>
    publicRequest<T>(endpoint, {
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }),
}

export const apiClient = {
  get: <T>(endpoint: string) => request<T>(endpoint, { method: 'GET' }),

  post: <T>(endpoint: string, data?: unknown) =>
    request<T>(endpoint, {
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }),

  put: <T>(endpoint: string, data: unknown) =>
    request<T>(endpoint, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  patch: <T>(endpoint: string, data: unknown) =>
    request<T>(endpoint, {
      method: 'PATCH',
      body: JSON.stringify(data),
    }),

  delete: <T>(endpoint: string) => request<T>(endpoint, { method: 'DELETE' }),

  download: async (endpoint: string, fallbackFilename: string): Promise<void> => {
    const response = await _authenticatedFetch(endpoint, {
      method: 'GET',
      timeout: UPLOAD_TIMEOUT_MS, // reuse upload timeout for large exports
      skipContentType: true,
    })

    // Prefer server-sanitized filename from Content-Disposition header.
    const disposition = response.headers.get('Content-Disposition')
    let resolvedFilename = fallbackFilename
    if (disposition) {
      const match = disposition.match(/filename="([^"]+)"/)
      if (match?.[1]) {
        resolvedFilename = match[1]
      }
    }

    const blob = await response.blob()
    const objectUrl = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = resolvedFilename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)

    // Delay revocation to avoid racing with browser download initiation
    setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000)
  },

  getStreamUrl: (id: string) =>
    request<MediaStreamUrlResponse>(`/media/${id}/stream-url`, { method: 'GET' }),

  upload: async <T>(
    endpoint: string,
    formData: FormData,
    onProgress?: (pct: number) => void,
    xhrFactory?: () => XMLHttpRequest
  ) => {
    // One decision point for both transports: media uploads and recording
    // chunks leave through the upload origin when one is configured.
    const baseUrl = resolveRequestBaseUrl(endpoint)

    if (!onProgress) {
      return request<T>(endpoint, {
        method: 'POST',
        body: formData,
        timeout: UPLOAD_TIMEOUT_MS,
        skipContentType: true,
        baseUrl,
      })
    }

    const tokenValid = await ensureValidToken()

    if (!tokenValid && !keycloak.token) {
      if (import.meta.env.DEV) {
        console.error('No valid token available, redirecting to login')
      }
      keycloak.login()
      throw new ApiError(401, 'Authentication required')
    }

    const url = `${baseUrl}${endpoint}`

    return new Promise<T>((resolve, reject) => {
      const createXHR = xhrFactory ?? (() => new XMLHttpRequest())
      const xhr = createXHR()
      const timeoutId = setTimeout(() => {
        xhr.abort()
      }, UPLOAD_TIMEOUT_MS)

      const parseResponse = () => {
        if (!xhr.responseText) return null
        try {
          return JSON.parse(xhr.responseText)
        } catch {
          return null
        }
      }

      xhr.open('POST', url)
      xhr.setRequestHeader('Authorization', `Bearer ${keycloak.token}`)

      xhr.upload.onprogress = (evt) => {
        if (evt.lengthComputable) {
          onProgress(Math.round((evt.loaded / evt.total) * 100))
        }
      }

      xhr.onload = () => {
        clearTimeout(timeoutId)
        if (xhr.status === 401) {
          if (import.meta.env.DEV) {
            console.error('Session expired, redirecting to login')
          }
          keycloak.login()
          reject(new ApiError(401, 'Session expired', parseResponse()))
          return
        }
        if (xhr.status < 200 || xhr.status >= 300) {
          const error = new ApiError(xhr.status, xhr.statusText || 'Upload failed', parseResponse())
          error.retryAfter = parseRetryAfterSeconds(xhr.getResponseHeader('Retry-After'))
          noteAccessDenial(error)
          reject(error)
          return
        }
        resolve(parseResponse() as T)
      }

      xhr.onerror = () => {
        clearTimeout(timeoutId)
        reject(new ApiError(0, 'Network error'))
      }

      xhr.onabort = () => {
        clearTimeout(timeoutId)
        reject(new ApiError(408, 'Request timeout'))
      }

      xhr.send(formData)
    })
  },
}
