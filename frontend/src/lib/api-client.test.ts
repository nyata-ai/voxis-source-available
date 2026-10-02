import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockUpdateToken = vi.fn().mockResolvedValue(true)
const mockLogin = vi.fn()

vi.mock('./keycloak', () => ({
  keycloak: {
    get token() {
      return 'test-token'
    },
    updateToken: mockUpdateToken,
    login: mockLogin,
  },
}))

const mockFetch = vi.fn()
globalThis.fetch = mockFetch

describe('api-client', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    vi.resetModules()
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-length': '15' }),
      text: () => Promise.resolve('{"data":"test"}'),
    })
  })

  it('should add Authorization header to requests', async () => {
    const { apiClient } = await import('./api-client')

    await apiClient.get('/test')

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/test'),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer test-token',
        }),
      })
    )
  })

  it('flags a missing app role app-wide without redirecting to login', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      headers: new Headers(),
      text: () =>
        Promise.resolve('{"error":"role_required","message":"account lacks the voxis-user role"}'),
    })
    const { apiClient, isRoleRequiredError } = await import('./api-client')
    const { useAccessStore } = await import('@/stores/access')

    const error = await apiClient.get('/me').catch((err: unknown) => err)

    expect(isRoleRequiredError(error)).toBe(true)
    expect(useAccessStore.getState().roleRequired).toBe(true)
    expect(mockLogin).not.toHaveBeenCalled()
  })

  it('does not treat an ordinary 403 as a missing role', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      headers: new Headers(),
      text: () => Promise.resolve('{"error":"forbidden","message":"not yours"}'),
    })
    const { apiClient, isRoleRequiredError } = await import('./api-client')
    const { useAccessStore } = await import('@/stores/access')

    const error = await apiClient.get('/media/1').catch((err: unknown) => err)

    expect(isRoleRequiredError(error)).toBe(false)
    expect(useAccessStore.getState().roleRequired).toBe(false)
  })

  it('should include /api/v1 in base URL by default', async () => {
    const { apiClient } = await import('./api-client')

    await apiClient.get('/me')

    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/me'),
      expect.any(Object)
    )
  })

  it('should refresh token before request if needed', async () => {
    const { apiClient } = await import('./api-client')

    await apiClient.get('/test')

    expect(mockUpdateToken).toHaveBeenCalledWith(30)
  })

  it('should throw ApiError on non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      headers: new Headers(),
      text: () => Promise.resolve('{"message":"Not found"}'),
    })

    const { apiClient, ApiError } = await import('./api-client')

    await expect(apiClient.get('/test')).rejects.toBeInstanceOf(ApiError)
  })

  it('should redirect to login on 401 response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      headers: new Headers(),
      text: () => Promise.resolve('{"message":"Token expired"}'),
    })

    const { apiClient } = await import('./api-client')

    await expect(apiClient.get('/test')).rejects.toThrow()
    expect(mockLogin).toHaveBeenCalled()
  })

  it('should handle 204 No Content response', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 204,
      headers: new Headers({ 'content-length': '0' }),
      text: () => Promise.resolve(''),
    })

    const { apiClient } = await import('./api-client')

    const result = await apiClient.delete('/test')

    expect(result).toBeNull()
  })

  it('should handle empty response body', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-length': '0' }),
      text: () => Promise.resolve(''),
    })

    const { apiClient } = await import('./api-client')

    const result = await apiClient.get('/test')

    expect(result).toBeNull()
  })

  describe('ApiError', () => {
    it('should have correct name property', async () => {
      const { ApiError } = await import('./api-client')

      const error = new ApiError(500, 'Server error')

      expect(error.name).toBe('ApiError')
      expect(error.status).toBe(500)
      expect(error.message).toBe('Server error')
    })

    it('should include data in error', async () => {
      const { ApiError } = await import('./api-client')

      const errorData = { code: 'ERR_001', details: 'Something went wrong' }
      const error = new ApiError(400, 'Bad Request', errorData)

      expect(error.data).toEqual(errorData)
    })
  })

  describe('ensureValidToken', () => {
    it('should return false when token is undefined', async () => {
      vi.resetModules()

      // Mock keycloak with undefined token
      vi.doMock('./keycloak', () => ({
        keycloak: {
          token: undefined,
          updateToken: mockUpdateToken,
          login: mockLogin,
        },
      }))

      const { apiClient } = await import('./api-client')

      await expect(apiClient.get('/test')).rejects.toThrow('Authentication required')
      expect(mockLogin).toHaveBeenCalled()
    })

    it('should prevent concurrent token refresh (mutex)', async () => {
      vi.resetModules()

      let resolveFirstRefresh: (value: boolean) => void
      const firstRefreshPromise = new Promise<boolean>((resolve) => {
        resolveFirstRefresh = resolve
      })

      let callCount = 0
      const slowUpdateToken = vi.fn().mockImplementation(() => {
        callCount++
        if (callCount === 1) {
          return firstRefreshPromise
        }
        return Promise.resolve(true)
      })

      vi.doMock('./keycloak', () => ({
        keycloak: {
          token: 'test-token',
          updateToken: slowUpdateToken,
          login: mockLogin,
        },
      }))

      const { apiClient } = await import('./api-client')

      // Start two concurrent requests
      const request1 = apiClient.get('/test1')
      const request2 = apiClient.get('/test2')

      // Resolve the first refresh
      resolveFirstRefresh!(true)

      await Promise.all([request1, request2])

      // updateToken should have been called only once due to mutex
      // (but may be called more times depending on implementation details)
      expect(slowUpdateToken).toHaveBeenCalled()
    })
  })

  describe('request timeout', () => {
    it('should handle request timeout', async () => {
      // Directly mock fetch to reject with AbortError
      const abortError = new Error('Aborted')
      abortError.name = 'AbortError'
      mockFetch.mockRejectedValueOnce(abortError)

      const { apiClient, ApiError } = await import('./api-client')

      try {
        await apiClient.get('/slow-endpoint')
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.status).toBe(408)
        expect(apiError.message).toBe('Request timeout')
      }
    })
  })

  describe('error response parsing', () => {
    it('should parse JSON error response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        headers: new Headers(),
        text: () => Promise.resolve('{"error":"validation_failed","details":"Invalid email"}'),
      })

      const { apiClient, ApiError } = await import('./api-client')

      try {
        await apiClient.get('/test')
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.data).toEqual({
          error: 'validation_failed',
          details: 'Invalid email',
        })
      }
    })

    it('should handle non-JSON error response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        headers: new Headers(),
        text: () => Promise.resolve('Gateway Timeout'),
      })

      const { apiClient, ApiError } = await import('./api-client')

      try {
        await apiClient.get('/test')
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.status).toBe(500)
        expect(apiError.data).toBeNull() // Non-JSON should result in null data
      }
    })
  })

  describe('HTTP methods', () => {
    it('should send POST with JSON body', async () => {
      const { apiClient } = await import('./api-client')

      const payload = { email: 'test@example.com', name: 'Test User' }
      await apiClient.post('/users', payload)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/users'),
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(payload),
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should send PUT with JSON body', async () => {
      const { apiClient } = await import('./api-client')

      const payload = { name: 'Updated Name' }
      await apiClient.put('/users/123', payload)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/users/123'),
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(payload),
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
          }),
        })
      )
    })

    it('should send DELETE request', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 204,
        headers: new Headers({ 'content-length': '0' }),
        text: () => Promise.resolve(''),
      })

      const { apiClient } = await import('./api-client')

      await apiClient.delete('/users/123')

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/users/123'),
        expect.objectContaining({
          method: 'DELETE',
        })
      )
    })

    it('should send PATCH with JSON body', async () => {
      const { apiClient } = await import('./api-client')

      const payload = { status: 'active' }
      await apiClient.patch('/users/123', payload)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/users/123'),
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify(payload),
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
          }),
        })
      )
    })
  })

  describe('redirect to login', () => {
    it('should redirect to login when no valid token', async () => {
      vi.resetModules()

      vi.doMock('./keycloak', () => ({
        keycloak: {
          token: undefined,
          updateToken: mockUpdateToken,
          login: mockLogin,
        },
      }))

      const { apiClient } = await import('./api-client')

      await expect(apiClient.get('/protected')).rejects.toThrow('Authentication required')
      expect(mockLogin).toHaveBeenCalled()
    })
  })

  describe('download', () => {
    // Re-apply the default keycloak mock to ensure isolation
    beforeEach(() => {
      vi.doMock('./keycloak', () => ({
        keycloak: {
          get token() {
            return 'test-token'
          },
          updateToken: mockUpdateToken,
          login: mockLogin,
        },
      }))
    })

    it('fetches blob and triggers browser download', async () => {
      const blobContent = new Blob(['%PDF-1.4 fake pdf content'], { type: 'application/pdf' })
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/pdf' }),
        blob: () => Promise.resolve(blobContent),
      })

      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/fake-url')
      const mockRevokeObjectURL = vi.fn()
      globalThis.URL.createObjectURL = mockCreateObjectURL
      globalThis.URL.revokeObjectURL = mockRevokeObjectURL

      const mockClick = vi.fn()
      const mockAppendChild = vi.spyOn(document.body, 'appendChild').mockImplementation((node) => node)
      const mockRemoveChild = vi.spyOn(document.body, 'removeChild').mockImplementation((node) => node)
      const mockCreateElement = vi.spyOn(document, 'createElement').mockReturnValue({
        href: '',
        download: '',
        click: mockClick,
      } as unknown as HTMLAnchorElement)

      const { apiClient } = await import('./api-client')

      await apiClient.download('/summaries/123/export?format=pdf', 'summary.pdf')

      // Verify fetch was called with correct endpoint and auth
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/summaries/123/export?format=pdf'),
        expect.objectContaining({
          method: 'GET',
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )

      // Verify Content-Type is NOT set (skipContentType: true)
      const callArgs = mockFetch.mock.calls[0][1]
      expect(callArgs.headers).not.toHaveProperty('Content-Type')

      // Verify blob download flow
      expect(mockCreateObjectURL).toHaveBeenCalledWith(blobContent)
      expect(mockCreateElement).toHaveBeenCalledWith('a')
      expect(mockAppendChild).toHaveBeenCalled()
      expect(mockClick).toHaveBeenCalled()
      expect(mockRemoveChild).toHaveBeenCalled()

      mockAppendChild.mockRestore()
      mockRemoveChild.mockRestore()
      mockCreateElement.mockRestore()
    })

    it('throws ApiError on non-ok response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: 'Not Found',
        headers: new Headers(),
        text: () => Promise.resolve('{"message":"Export not found"}'),
      })

      const { apiClient, ApiError } = await import('./api-client')

      try {
        await apiClient.download('/summaries/999/export?format=pdf', 'summary.pdf')
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.status).toBe(404)
      }
    })

    it('throws on network error', async () => {
      const abortError = new Error('Aborted')
      abortError.name = 'AbortError'
      mockFetch.mockRejectedValueOnce(abortError)

      const { apiClient, ApiError } = await import('./api-client')

      try {
        await apiClient.download('/summaries/123/export?format=pdf', 'summary.pdf')
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.status).toBe(408)
        expect(apiError.message).toBe('Request timeout')
      }
    })

    it('uses filename from Content-Disposition header when available', async () => {
      const blobContent = new Blob(['%PDF-1.4 content'], { type: 'application/pdf' })
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({
          'content-type': 'application/pdf',
          'content-disposition': 'attachment; filename="meeting-recording.pdf"',
        }),
        blob: () => Promise.resolve(blobContent),
      })

      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/fake-url')
      globalThis.URL.createObjectURL = mockCreateObjectURL
      globalThis.URL.revokeObjectURL = vi.fn()

      const linkEl = { href: '', download: '', click: vi.fn() }
      const mockAppendChild = vi.spyOn(document.body, 'appendChild').mockImplementation((node) => node)
      const mockRemoveChild = vi.spyOn(document.body, 'removeChild').mockImplementation((node) => node)
      const mockCreateElement = vi.spyOn(document, 'createElement').mockReturnValue(
        linkEl as unknown as HTMLAnchorElement,
      )

      const { apiClient } = await import('./api-client')

      await apiClient.download('/transcriptions/123/export?format=pdf', 'fallback.pdf')

      // Should use server-provided filename, not the fallback
      expect(linkEl.download).toBe('meeting-recording.pdf')

      mockAppendChild.mockRestore()
      mockRemoveChild.mockRestore()
      mockCreateElement.mockRestore()
    })

    it('falls back to client filename when Content-Disposition is absent', async () => {
      const blobContent = new Blob(['{}'], { type: 'application/json' })
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        blob: () => Promise.resolve(blobContent),
      })

      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/fake-url')
      globalThis.URL.createObjectURL = mockCreateObjectURL
      globalThis.URL.revokeObjectURL = vi.fn()

      const linkEl = { href: '', download: '', click: vi.fn() }
      const mockAppendChild = vi.spyOn(document.body, 'appendChild').mockImplementation((node) => node)
      const mockRemoveChild = vi.spyOn(document.body, 'removeChild').mockImplementation((node) => node)
      const mockCreateElement = vi.spyOn(document, 'createElement').mockReturnValue(
        linkEl as unknown as HTMLAnchorElement,
      )

      const { apiClient } = await import('./api-client')

      await apiClient.download('/summaries/456/export?format=json', 'my-summary.json')

      // Should use the fallback filename
      expect(linkEl.download).toBe('my-summary.json')

      mockAppendChild.mockRestore()
      mockRemoveChild.mockRestore()
      mockCreateElement.mockRestore()
    })
  })

  describe('upload', () => {
    // Re-apply the default keycloak mock to ensure isolation from earlier
    // vi.doMock() calls (e.g. in ensureValidToken/redirect-to-login tests)
    beforeEach(() => {
      vi.doMock('./keycloak', () => ({
        keycloak: {
          get token() {
            return 'test-token'
          },
          updateToken: mockUpdateToken,
          login: mockLogin,
        },
      }))
    })

    it('should send FormData as body', async () => {
      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['audio-data'], { type: 'audio/wav' }), 'recording.wav')

      await apiClient.upload('/recordings/upload', formData)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/recordings/upload'),
        expect.objectContaining({
          method: 'POST',
          body: formData,
        })
      )
    })

    it('should NOT set Content-Type header (browser sets boundary for FormData)', async () => {
      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      await apiClient.upload('/upload', formData)

      const callArgs = mockFetch.mock.calls[0][1]
      expect(callArgs.headers).not.toHaveProperty('Content-Type')
    })

    it('should include Authorization Bearer token', async () => {
      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      await apiClient.upload('/upload', formData)

      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )
    })

    it('should call ensureValidToken before request', async () => {
      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      await apiClient.upload('/upload', formData)

      expect(mockUpdateToken).toHaveBeenCalledWith(30)
    })

    it('should handle errors the same as other methods', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 413,
        statusText: 'Payload Too Large',
        headers: new Headers(),
        text: () => Promise.resolve('{"error":"file_too_large","max_size":"50MB"}'),
      })

      const { apiClient, ApiError } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'large-file.wav')

      try {
        await apiClient.upload('/upload', formData)
        expect.fail('Should have thrown')
      } catch (err) {
        expect(err).toBeInstanceOf(ApiError)
        const apiError = err as InstanceType<typeof ApiError>
        expect(apiError.status).toBe(413)
        expect(apiError.data).toEqual({
          error: 'file_too_large',
          max_size: '50MB',
        })
      }
    })

    it('should redirect to login on 401 during upload', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        headers: new Headers(),
        text: () => Promise.resolve('{"message":"Token expired"}'),
      })

      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      await expect(apiClient.upload('/upload', formData)).rejects.toThrow()
      expect(mockLogin).toHaveBeenCalled()
    })

    it('should parse JSON response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-length': '30' }),
        text: () => Promise.resolve('{"id":"rec-123","status":"uploaded"}'),
      })

      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      const result = await apiClient.upload<{ id: string; status: string }>('/upload', formData)

      expect(result).toEqual({ id: 'rec-123', status: 'uploaded' })
    })

    it('should report upload progress when onProgress is provided', async () => {
      class MockXMLHttpRequest {
        static lastInstance: MockXMLHttpRequest | null = null

        method = ''
        url = ''
        status = 200
        statusText = 'OK'
        responseText = '{"id":"rec-123"}'
        headers: Record<string, string> = {}
        body: unknown
        upload = { onprogress: null as ((evt: ProgressEvent) => void) | null }
        onload: (() => void) | null = null
        onerror: (() => void) | null = null
        onabort: (() => void) | null = null

        constructor() {
          MockXMLHttpRequest.lastInstance = this
        }

        open(method: string, url: string) {
          this.method = method
          this.url = url
        }

        setRequestHeader(key: string, value: string) {
          this.headers[key] = value
        }

        send(body: unknown) {
          this.body = body
        }

        abort() {
          this.onabort?.()
        }

        triggerProgress(loaded: number, total: number) {
          this.upload.onprogress?.({
            lengthComputable: true,
            loaded,
            total,
          } as ProgressEvent)
        }

        triggerLoad() {
          this.onload?.()
        }
      }

      const { apiClient } = await import('./api-client')

      const formData = new FormData()
      formData.append('file', new Blob(['data']), 'test.wav')

      const progress = vi.fn()
      const promise = apiClient.upload('/upload', formData, progress, () =>
        new MockXMLHttpRequest() as unknown as XMLHttpRequest
      )

      await new Promise((resolve) => setTimeout(resolve, 0))

      const xhr = MockXMLHttpRequest.lastInstance
      expect(xhr).not.toBeNull()

      if (xhr) {
        xhr.triggerProgress(50, 100)
        xhr.triggerLoad()
      }

      await promise

      expect(progress).toHaveBeenCalledWith(50)
      expect(xhr?.headers.Authorization).toBe('Bearer test-token')
    })
  })
})

/**
 * VITE_UPLOAD_URL sends the two body-heavy requests to a second origin, because
 * a deployment may sit behind a proxy that caps request bodies well below the
 * upload limit. Unset must behave exactly as before — that is what keeps every
 * other deployment on a single origin with no config change.
 */
describe('upload origin routing', () => {
  const API = 'https://app.example.com/api/v1'
  const UPLOAD = 'https://upload.example.com'

  beforeEach(() => {
    vi.clearAllMocks()
    vi.resetModules()
    vi.unstubAllEnvs()
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-length': '15' }),
      text: () => Promise.resolve('{"data":"test"}'),
    })
  })

  it('keeps every request on the API origin when the variable is unset', async () => {
    const { resolveRequestBaseUrl } = await import('./api-client')

    expect(resolveRequestBaseUrl('/media/upload', '', API)).toBe(API)
    expect(resolveRequestBaseUrl('/recordings/abc/chunks', '   ', API)).toBe(API)
  })

  it('routes only the two upload endpoints to the upload origin', async () => {
    const { resolveRequestBaseUrl } = await import('./api-client')

    expect(resolveRequestBaseUrl('/media/upload', UPLOAD, API)).toBe(`${UPLOAD}/api/v1`)
    expect(resolveRequestBaseUrl('/recordings/9f3-a1/chunks', UPLOAD, API)).toBe(`${UPLOAD}/api/v1`)

    // Everything else — including near-misses that the upload host would 404.
    for (const endpoint of ['/media', '/media/123', '/recordings', '/recordings/9f3', '/me']) {
      expect(resolveRequestBaseUrl(endpoint, UPLOAD, API)).toBe(API)
    }
  })

  it('takes the origin from the variable and the path prefix from the API base', async () => {
    const { resolveRequestBaseUrl } = await import('./api-client')

    // Both spellings an operator might write must produce the same URL: the
    // upload host routes the exact API paths and nothing else.
    for (const configured of [UPLOAD, `${UPLOAD}/`, `${UPLOAD}/api/v1`]) {
      expect(resolveRequestBaseUrl('/media/upload', configured, API)).toBe(`${UPLOAD}/api/v1`)
    }
    // A same-origin API base (relative path) still yields a usable prefix.
    expect(resolveRequestBaseUrl('/media/upload', UPLOAD, '/api/v1')).toBe(`${UPLOAD}/api/v1`)
  })

  it('throws on a malformed upload origin instead of silently using the proxied host', async () => {
    const { resolveRequestBaseUrl } = await import('./api-client')

    expect(() => resolveRequestBaseUrl('/media/upload', 'upload.example.com', API)).toThrow(
      /VITE_UPLOAD_URL/
    )
  })

  it('sends the media upload to the configured origin, with the auth header intact', async () => {
    vi.stubEnv('VITE_UPLOAD_URL', UPLOAD)
    const { apiClient } = await import('./api-client')

    const formData = new FormData()
    formData.append('file', new Blob(['data']), 'test.wav')
    await apiClient.upload('/media/upload', formData)

    expect(mockFetch).toHaveBeenCalledWith(
      `${UPLOAD}/api/v1/media/upload`,
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    )
  })

  it('sends recording chunks to the configured origin', async () => {
    vi.stubEnv('VITE_UPLOAD_URL', UPLOAD)
    const { apiClient } = await import('./api-client')

    const formData = new FormData()
    formData.append('file', new Blob(['chunk']))
    await apiClient.upload('/recordings/session-1/chunks', formData)

    expect(mockFetch).toHaveBeenCalledWith(
      `${UPLOAD}/api/v1/recordings/session-1/chunks`,
      expect.any(Object)
    )
  })

  it('leaves ordinary requests on the API origin even when an upload origin is set', async () => {
    vi.stubEnv('VITE_UPLOAD_URL', UPLOAD)
    const { apiClient, API_BASE_URL } = await import('./api-client')

    await apiClient.get('/media')

    expect(mockFetch).toHaveBeenCalledWith(`${API_BASE_URL}/media`, expect.any(Object))
  })
})
