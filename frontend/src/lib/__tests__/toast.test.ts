import { describe, it, expect, vi } from 'vitest'
import { toast as sonnerToast } from 'sonner'

vi.mock('sonner', () => {
  const mockToast = Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
    loading: vi.fn(),
    dismiss: vi.fn(),
  })
  return { toast: mockToast }
})

import { toast } from '../toast'

describe('toast utility', () => {
  it('delegates success to sonner', () => {
    toast.success('Saved')
    expect(sonnerToast.success).toHaveBeenCalledWith('Saved')
  })

  it('delegates error to sonner', () => {
    toast.error('Failed')
    expect(sonnerToast.error).toHaveBeenCalledWith('Failed')
  })

  it('delegates info to sonner', () => {
    toast.info('Note')
    expect(sonnerToast).toHaveBeenCalledWith('Note')
  })

  it('delegates loading to sonner', () => {
    toast.loading('Working...')
    expect(sonnerToast.loading).toHaveBeenCalledWith('Working...')
  })

  it('delegates dismiss to sonner', () => {
    toast.dismiss()
    expect(sonnerToast.dismiss).toHaveBeenCalled()
  })
})
