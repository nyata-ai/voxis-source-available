import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface AdminLLMPrompt {
  key: string
  label: string
  summary_type: string
  prompt_version: string
  system_instruction: string
  user_prompt: string
  model: string
  has_override: boolean
  updated_at?: string
  runtime?: string
  revision?: string
  quantization?: string
  usage_available?: boolean
}
export interface UpdateAdminLLMPromptRequest {
  key: string
  system_instruction: string
  user_prompt: string
  model: string
}
const keys = {
  all: ['admin', 'llm', 'prompts'] as const,
  list: () => [...keys.all, 'list'] as const,
}
export function useAdminLLMPrompts() {
  return useQuery({
    queryKey: keys.list(),
    queryFn: async () =>
      (await apiClient.get<{ prompts: AdminLLMPrompt[] }>('/admin/llm/prompts')).prompts,
    staleTime: 30_000,
  })
}
export function useUpdateAdminLLMPrompt() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ key, ...payload }: UpdateAdminLLMPromptRequest) =>
      apiClient.put<AdminLLMPrompt>(`/admin/llm/prompts/${encodeURIComponent(key)}`, payload),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: keys.list() }),
  })
}
export function useResetAdminLLMPrompt() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) =>
      apiClient.delete<AdminLLMPrompt>(`/admin/llm/prompts/${encodeURIComponent(key)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: keys.list() }),
  })
}
