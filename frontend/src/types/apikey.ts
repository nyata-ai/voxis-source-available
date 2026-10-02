export interface APIKey {
  id: string
  name: string
  key_prefix: string
  scopes: string[]
  last_used_at: string | null
  expires_at: string | null
  created_at: string
}

export interface CreateAPIKeyRequest {
  name: string
  scopes: string[]
  expires_in_days?: number
}

export interface CreateAPIKeyResponse {
  id: string
  name: string
  key: string
  key_prefix: string
  scopes: string[]
  mcp_url: string
  api_url: string
  expires_at: string | null
  created_at: string
}

export const AVAILABLE_SCOPES = [
  {
    value: 'media:read',
    labelKey: 'apiKeys.scopeOptions.mediaRead.label',
    descriptionKey: 'apiKeys.scopeOptions.mediaRead.description',
  },
  {
    value: 'media:write',
    labelKey: 'apiKeys.scopeOptions.mediaWrite.label',
    descriptionKey: 'apiKeys.scopeOptions.mediaWrite.description',
  },
  {
    value: 'transcription:read',
    labelKey: 'apiKeys.scopeOptions.transcriptionRead.label',
    descriptionKey: 'apiKeys.scopeOptions.transcriptionRead.description',
  },
  {
    value: 'transcription:write',
    labelKey: 'apiKeys.scopeOptions.transcriptionWrite.label',
    descriptionKey: 'apiKeys.scopeOptions.transcriptionWrite.description',
  },
  {
    value: 'summary:read',
    labelKey: 'apiKeys.scopeOptions.summaryRead.label',
    descriptionKey: 'apiKeys.scopeOptions.summaryRead.description',
  },
  {
    value: 'summary:write',
    labelKey: 'apiKeys.scopeOptions.summaryWrite.label',
    descriptionKey: 'apiKeys.scopeOptions.summaryWrite.description',
  },
  {
    value: 'export:read',
    labelKey: 'apiKeys.scopeOptions.exportRead.label',
    descriptionKey: 'apiKeys.scopeOptions.exportRead.description',
  },
] as const
