import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useOutletContext } from 'react-router-dom'
import { ApiError } from '@/lib/api-client'
import { useAdmin } from '@/hooks/useAdmin'
import { AdminRoute, type AdminOutletContext } from './AdminRoute'

vi.mock('@/hooks/useAdmin', () => ({
  useAdmin: vi.fn(),
}))

function renderAdminRoute() {
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="/admin" element={<div>Admin child</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

function AdminChild() {
  const { admin } = useOutletContext<AdminOutletContext>()
  return <div>{admin.admin ? 'admin confirmed' : 'admin denied'}</div>
}

describe('AdminRoute', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders admin children when the admin API confirms access', () => {
    vi.mocked(useAdmin).mockReturnValue({
      data: { admin: true },
      error: null,
      isError: false,
      isLoading: false,
    } as unknown as ReturnType<typeof useAdmin>)

    renderAdminRoute()

    expect(screen.getByText('Admin child')).toBeInTheDocument()
  })

  it('passes only the verified admin capability to child routes', () => {
    vi.mocked(useAdmin).mockReturnValue({
      data: { admin: true },
      error: null,
      isError: false,
      isLoading: false,
    } as unknown as ReturnType<typeof useAdmin>)

    render(
      <MemoryRouter initialEntries={['/admin']}>
        <Routes>
          <Route element={<AdminRoute />}>
            <Route path="/admin" element={<AdminChild />} />
          </Route>
        </Routes>
      </MemoryRouter>
    )

    expect(screen.getByText('admin confirmed')).toBeInTheDocument()
    expect(screen.queryByText('admin-user')).not.toBeInTheDocument()
  })

  it('renders a forbidden state for non-admin callers', () => {
    vi.mocked(useAdmin).mockReturnValue({
      data: undefined,
      error: new ApiError(403, 'Forbidden'),
      isError: true,
      isLoading: false,
    } as unknown as ReturnType<typeof useAdmin>)

    renderAdminRoute()

    expect(screen.getByRole('heading', { name: 'Admin access required' })).toBeInTheDocument()
    expect(screen.queryByText('Admin child')).not.toBeInTheDocument()
  })
})
