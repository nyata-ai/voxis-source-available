import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'
import type { APIKey, CreateAPIKeyRequest, CreateAPIKeyResponse } from '@/types/apikey'

export const apiKeyKeys = {
  all: ['api-keys'] as const,
  list: () => [...apiKeyKeys.all, 'list'] as const,
}

export function useApiKeys() {
  return useQuery({
    queryKey: apiKeyKeys.list(),
    queryFn: async () => {
      const data = await apiClient.get<{ keys: APIKey[] }>('/api-keys')
      return data.keys
    },
  })
}

export function useCreateApiKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateAPIKeyRequest) =>
      apiClient.post<CreateAPIKeyResponse>('/api-keys', req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.list() })
    },
  })
}

export function useRevokeApiKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/api-keys/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.list() })
    },
  })
}
