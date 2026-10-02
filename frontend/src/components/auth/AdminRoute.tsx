import { Outlet } from 'react-router-dom'
import { AlertTriangle, ShieldAlert } from 'lucide-react'
import { ApiError } from '@/lib/api-client'
import { useAdmin } from '@/hooks/useAdmin'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { AdminMe } from '@/hooks/useAdmin'

export interface AdminOutletContext {
  admin: AdminMe
}

export function AdminRoute() {
  const { data, error, isError, isLoading } = useAdmin()

  if (isLoading) {
    return (
      <div className="space-y-2">
        <h1 className="text-2xl font-normal">Admin</h1>
        <p className="text-muted-foreground">Checking admin access...</p>
      </div>
    )
  }

  const forbidden = (isError && error instanceof ApiError && error.status === 403) || data?.admin === false

  if (forbidden) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Card className="w-full max-w-md text-center">
          <CardHeader>
            <ShieldAlert className="mx-auto mb-2 h-10 w-10 text-muted-foreground" aria-hidden="true" />
            <CardTitle>Admin access required</CardTitle>
            <CardDescription>You do not have permission to view this page.</CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Card className="w-full max-w-md text-center">
          <CardHeader>
            <AlertTriangle className="mx-auto mb-2 h-10 w-10 text-muted-foreground" aria-hidden="true" />
            <CardTitle>Unable to load admin status</CardTitle>
            <CardDescription>Try again later.</CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return <Outlet context={{ admin: data } satisfies AdminOutletContext} />
}
