import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'

const usePrompts = vi.fn()
const useUpdate = vi.fn()
const useReset = vi.fn()

vi.mock('@/components/admin/AdminRecordingRetentionPanel', () => ({
  AdminRecordingRetentionPanel: () => <div>Retention control</div>,
}))
vi.mock('@/components/admin/AdminStorageQuotaPanel', () => ({
  AdminStorageQuotaPanel: () => <div>Storage control</div>,
}))
vi.mock('@/hooks/useAdminSystemStats', () => ({
  useAdminSystemStats: () => ({
    data: {
      build: { version: 'test', commit: 'abc' },
      storage: { backend: 'local', available: true },
      dependencies: [],
    },
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
  }),
}))
vi.mock('@/hooks/useAdminOpsStats', () => ({
  useAdminOpsStats: () => ({
    data: {
      media: { total: 1, by_status: {} },
      scan: { total: 1, by_status: {} },
      recordings: { total: 1, by_status: {} },
      transcriptions: { total: 1, by_status: {}, stale_submitted_over_2h: 0 },
      summaries: { total: 1, by_status: {}, stale_pending_over_30m: 0 },
      providers: {
        speechmatics: { completed_jobs: 1, failed_jobs: 0 },
        gemma: {
          completed_summaries: 1,
          usage_available: true,
          prompt_tokens: 1,
          completion_tokens: 1,
          thinking_tokens: 0,
          total_tokens: 2,
        },
      },
      summary_model: {
        model: 'google/gemma-4-12B-it',
        runtime: 'llama.cpp',
        revision: 'r1',
        quantization: 'q4_0',
        usage_available: true,
      },
    },
    isError: false,
  }),
}))
vi.mock('@/hooks/useAdminLLMPrompts', () => ({
  useAdminLLMPrompts: () => usePrompts(),
  useUpdateAdminLLMPrompt: () => useUpdate(),
  useResetAdminLLMPrompt: () => useReset(),
}))

import { AdminPage } from './AdminPage'

const prompt = {
  key: 'general',
  label: 'General summary',
  summary_type: 'general',
  prompt_version: 'v1',
  system_instruction: 'system',
  user_prompt: 'user',
  model: 'google/gemma-4-12B-it',
  has_override: true,
}

function renderPage() {
  return render(<AdminPage />)
}

describe('Voxis-OSS administration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    usePrompts.mockReturnValue({ data: [], isLoading: false, isError: false })
    useUpdate.mockReturnValue({ isPending: false, mutateAsync: vi.fn() })
    useReset.mockReturnValue({ isPending: false, mutateAsync: vi.fn() })
  })

  it('shows operational controls and the served local model metadata', () => {
    renderPage()
    // Brand-agnostic: the product name in this sentence comes from oss.json.
    expect(
      screen.getByText(/^Operational controls for this .+ installation\.$/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Model:\s*google\/gemma-4-12B-it/)).toBeInTheDocument()
    expect(screen.getByText('Usage available: yes')).toBeInTheDocument()
    expect(screen.getByText('2 tokens reported')).toBeInTheDocument()
    expect(screen.getByText('Retention control')).toBeInTheDocument()
    expect(screen.getByText('Storage control')).toBeInTheDocument()
  })

  it('handles rejected prompt saves and resets with a visible error', async () => {
    const update = vi.fn().mockRejectedValue(new Error('offline'))
    const reset = vi.fn().mockRejectedValue(new Error('offline'))
    usePrompts.mockReturnValue({ data: [prompt], isLoading: false, isError: false })
    useUpdate.mockReturnValue({ isPending: false, mutateAsync: update })
    useReset.mockReturnValue({ isPending: false, mutateAsync: reset })
    renderPage()
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not save this prompt. Try again.'
    )
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not reset this prompt. Try again.'
    )
    expect(update).toHaveBeenCalledWith(expect.objectContaining({ key: 'general' }))
    expect(reset).toHaveBeenCalledWith('general')
  })
})
