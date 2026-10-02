import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { countAllUnsent } from '@/lib/chunk-outbox'
import { UserButton } from './UserButton'

const mockLogout = vi.fn().mockResolvedValue(undefined)

vi.mock('@/contexts/AuthContext', () => ({
  useAuth: () => ({
    user: { id: 'user-1', email: 'reader@example.com', name: 'Reader', username: 'reader' },
    logout: mockLogout,
  }),
}))

vi.mock('@/hooks/useSettings', () => ({
  useUpdatePreferences: vi.fn(() => ({ mutate: vi.fn() })),
}))

vi.mock('@/lib/chunk-outbox', () => ({
  countAllUnsent: vi.fn(),
}))

function renderButton() {
  return render(
    <MemoryRouter>
      <UserButton />
    </MemoryRouter>
  )
}

async function clickSignOut() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /account menu/i }))
  await user.click(screen.getByRole('menuitem', { name: /sign out/i }))
  return user
}

describe('UserButton sign-out', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('signs out straight away when no recording audio is waiting in the browser', async () => {
    vi.mocked(countAllUnsent).mockResolvedValue(0)
    renderButton()

    await clickSignOut()

    await waitFor(() => expect(mockLogout).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('warns before signing out when unsent recording audio would be deleted', async () => {
    vi.mocked(countAllUnsent).mockResolvedValue(2)
    renderButton()

    const user = await clickSignOut()

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(/not been uploaded/i)
    expect(mockLogout).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /sign out and delete/i }))
    expect(mockLogout).toHaveBeenCalledTimes(1)
  })

  it('keeps the user signed in when they choose to stay', async () => {
    vi.mocked(countAllUnsent).mockResolvedValue(1)
    renderButton()

    const user = await clickSignOut()
    await screen.findByRole('alertdialog')
    await user.click(screen.getByRole('button', { name: /stay signed in/i }))

    expect(mockLogout).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('still signs out when the browser store cannot be read', async () => {
    vi.mocked(countAllUnsent).mockRejectedValue(new Error('IndexedDB unavailable'))
    renderButton()

    await clickSignOut()

    await waitFor(() => expect(mockLogout).toHaveBeenCalledTimes(1))
  })
})
