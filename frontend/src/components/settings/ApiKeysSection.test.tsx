import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18n from '@/i18n'
import { ApiError } from '@/lib/api-client'
import { ApiKeysSection } from './ApiKeysSection'
import type { APIKey, CreateAPIKeyResponse } from '@/types/apikey'

vi.mock('@/hooks/useApiKeys', () => ({
  useApiKeys: vi.fn(),
  useCreateApiKey: vi.fn(),
  useRevokeApiKey: vi.fn(),
}))

import { useApiKeys, useCreateApiKey, useRevokeApiKey } from '@/hooks/useApiKeys'

const mockMutate = vi.fn()
const mockRevokeMutate = vi.fn()

function setupDefaultMocks(keys: APIKey[] = [], createError: Error | null = null) {
  vi.mocked(useApiKeys).mockReturnValue({
    data: keys,
    isLoading: false,
    isError: false,
    error: null,
  } as ReturnType<typeof useApiKeys>)

  vi.mocked(useCreateApiKey).mockReturnValue({
    mutateAsync: mockMutate,
    isPending: false,
    isError: createError !== null,
    error: createError,
  } as unknown as ReturnType<typeof useCreateApiKey>)

  vi.mocked(useRevokeApiKey).mockReturnValue({
    mutateAsync: mockRevokeMutate,
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useRevokeApiKey>)
}

describe('ApiKeysSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders section heading and description', () => {
    setupDefaultMocks()
    render(<ApiKeysSection />)

    expect(screen.getByText('API Keys')).toBeInTheDocument()
    expect(screen.getByText(/programmatic access/i)).toBeInTheDocument()
  })

  it('renders Create API Key button', () => {
    setupDefaultMocks()
    render(<ApiKeysSection />)

    expect(screen.getByRole('button', { name: /create api key/i })).toBeInTheDocument()
  })

  it('shows empty state when no keys exist', () => {
    setupDefaultMocks([])
    render(<ApiKeysSection />)

    expect(screen.getByText(/no api keys/i)).toBeInTheDocument()
  })

  it('renders key list with name, prefix, and revoke button', () => {
    const keys: APIKey[] = [
      {
        id: 'key-1',
        name: 'Production Key',
        key_prefix: 'vx_prod',
        scopes: ['media:read', 'transcription:read'],
        last_used_at: null,
        expires_at: null,
        created_at: '2026-02-10T10:00:00Z',
      },
      {
        id: 'key-2',
        name: 'Dev Key',
        key_prefix: 'vx_dev',
        scopes: ['media:read'],
        last_used_at: '2026-02-11T15:00:00Z',
        expires_at: '2026-05-10T10:00:00Z',
        created_at: '2026-02-09T10:00:00Z',
      },
    ]

    setupDefaultMocks(keys)
    render(<ApiKeysSection />)

    expect(screen.getByText('Production Key')).toBeInTheDocument()
    expect(screen.getByText('vx_prod...')).toBeInTheDocument()
    expect(screen.getByText('Dev Key')).toBeInTheDocument()
    expect(screen.getByText('vx_dev...')).toBeInTheDocument()

    const revokeButtons = screen.getAllByRole('button', { name: /revoke/i })
    expect(revokeButtons).toHaveLength(2)
  })

  it('shows "Never" for keys that have not been used', () => {
    const keys: APIKey[] = [
      {
        id: 'key-1',
        name: 'Unused Key',
        key_prefix: 'vx_test',
        scopes: ['media:read'],
        last_used_at: null,
        expires_at: null,
        created_at: '2026-02-10T10:00:00Z',
      },
    ]

    setupDefaultMocks(keys)
    render(<ApiKeysSection />)

    expect(screen.getByText(/Last used:.*Never/)).toBeInTheDocument()
  })

  it('opens create dialog when clicking Create API Key', async () => {
    const user = userEvent.setup()
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))

    expect(screen.getByText(/create new api key/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/key name/i)).toBeInTheDocument()
  })

  it('renders create dialog scope text from the locale catalog', async () => {
    const user = userEvent.setup()
    i18n.addResource('en', 'settings', 'apiKeys.scopeOptions.mediaRead.label', 'Localized Media Read')
    i18n.addResource('en', 'settings', 'apiKeys.scopeOptions.mediaRead.description', 'Localized media description')
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))

    expect(screen.getByText('Localized Media Read')).toBeInTheDocument()
    expect(screen.getByText(/Localized media description/)).toBeInTheDocument()
  })

  it('renders create dialog errors from stable API error codes', async () => {
    const user = userEvent.setup()
    setupDefaultMocks([], new ApiError(400, 'Bad Request', {
      error: 'validation_error',
      message: 'server-side English message',
    }))
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))

    expect(
      screen.getByText('Some details need attention. Please check the form and try again.')
    ).toBeInTheDocument()
    expect(screen.queryByText('server-side English message')).not.toBeInTheDocument()
  })

  it('creates key and shows created dialog with full key', async () => {
    const user = userEvent.setup()
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'My New Key',
      key: 'vx_abc123fullkey',
      key_prefix: 'vx_abc',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    mockMutate.mockResolvedValueOnce(mockResponse)
    setupDefaultMocks()
    render(<ApiKeysSection />)

    // Open create dialog
    await user.click(screen.getByRole('button', { name: /create api key/i }))

    // Fill in name
    await user.type(screen.getByLabelText(/key name/i), 'My New Key')

    // Submit
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    // Should show the created dialog with the key
    expect(await screen.findByText(/only be shown once/i)).toBeInTheDocument()
    expect(screen.getByText('vx_abc123fullkey')).toBeInTheDocument()
  })

  it('copies key to clipboard when clicking copy button', async () => {
    const user = userEvent.setup()
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'Clipboard Key',
      key: 'vx_clipboardkey',
      key_prefix: 'vx_clip',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    mockMutate.mockResolvedValueOnce(mockResponse)
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))
    await user.type(screen.getByLabelText(/key name/i), 'Clipboard Key')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    // Wait for created dialog
    await screen.findByText(/only be shown once/i)

    // Find copy button for the API key using aria-label and click it
    const copyButton = screen.getByRole('button', { name: /copy api key/i })
    await user.click(copyButton)

    // Verify the button text changed to "Copied!" which confirms the handler ran
    expect(await screen.findByText('Copied!')).toBeInTheDocument()
  })

  it('shows platform tabs in created dialog', async () => {
    const user = userEvent.setup()
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'Tab Key',
      key: 'vx_tabkey',
      key_prefix: 'vx_tab',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    mockMutate.mockResolvedValueOnce(mockResponse)
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))
    await user.type(screen.getByLabelText(/key name/i), 'Tab Key')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await screen.findByText(/only be shown once/i)

    // Verify platform tabs exist
    expect(screen.getByRole('tab', { name: /claude/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /cursor/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /rest api/i })).toBeInTheDocument()
  })

  it('switches platform tab content', async () => {
    const user = userEvent.setup()
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'Switch Key',
      key: 'vx_switchkey',
      key_prefix: 'vx_sw',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    mockMutate.mockResolvedValueOnce(mockResponse)
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))
    await user.type(screen.getByLabelText(/key name/i), 'Switch Key')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await screen.findByText(/only be shown once/i)

    // Click REST API tab
    await user.click(screen.getByRole('tab', { name: /rest api/i }))

    // Should show curl-related content in the active tab panel
    const restPanel = screen.getByRole('tabpanel')
    const curlElements = within(restPanel).getAllByText(/curl/i)
    expect(curlElements.length).toBeGreaterThan(0)
  })

  it('shows revoke confirmation and calls mutation on confirm', async () => {
    const user = userEvent.setup()
    mockRevokeMutate.mockResolvedValueOnce(null)

    const keys: APIKey[] = [
      {
        id: 'key-1',
        name: 'Revoke Me',
        key_prefix: 'vx_rev',
        scopes: ['media:read'],
        last_used_at: null,
        expires_at: null,
        created_at: '2026-02-10T10:00:00Z',
      },
    ]

    setupDefaultMocks(keys)
    render(<ApiKeysSection />)

    // Click revoke
    await user.click(screen.getByRole('button', { name: /revoke/i }))

    // Confirmation dialog should appear
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument()

    // Confirm the revocation
    await user.click(screen.getByRole('button', { name: /^revoke$/i }))

    expect(mockRevokeMutate).toHaveBeenCalledWith('key-1')
  })

  it('shows loading state', () => {
    vi.mocked(useApiKeys).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    } as ReturnType<typeof useApiKeys>)

    vi.mocked(useCreateApiKey).mockReturnValue({
      mutateAsync: mockMutate,
      isPending: false,
      isError: false,
      error: null,
    } as unknown as ReturnType<typeof useCreateApiKey>)

    vi.mocked(useRevokeApiKey).mockReturnValue({
      mutateAsync: mockRevokeMutate,
      isPending: false,
      isError: false,
      error: null,
    } as unknown as ReturnType<typeof useRevokeApiKey>)

    render(<ApiKeysSection />)

    const skeletons = document.querySelectorAll('.animate-pulse')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('closes created dialog with Done button', async () => {
    const user = userEvent.setup()
    const mockResponse: CreateAPIKeyResponse = {
      id: 'key-new',
      name: 'Done Key',
      key: 'vx_donekey',
      key_prefix: 'vx_done',
      scopes: ['media:read'],
      mcp_url: 'https://api.voxis.app/mcp',
      api_url: 'https://api.voxis.app/api/v1',
      expires_at: null,
      created_at: '2026-02-12T10:00:00Z',
    }

    mockMutate.mockResolvedValueOnce(mockResponse)
    setupDefaultMocks()
    render(<ApiKeysSection />)

    await user.click(screen.getByRole('button', { name: /create api key/i }))
    await user.type(screen.getByLabelText(/key name/i), 'Done Key')
    await user.click(screen.getByRole('button', { name: /^create$/i }))

    await screen.findByText(/only be shown once/i)

    // Click Done
    await user.click(screen.getByRole('button', { name: /done/i }))

    // Created dialog should close
    expect(screen.queryByText(/only be shown once/i)).not.toBeInTheDocument()
  })
})
