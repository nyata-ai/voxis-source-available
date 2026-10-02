import { create } from 'zustand'

interface AccessState {
  /** The server said this signed-in account lacks the app role (403 role_required). */
  roleRequired: boolean
  markRoleRequired: () => void
}

// Set by the API client the first time any request comes back 403
// `role_required`. ProtectedRoute then shows one "ask your administrator"
// screen instead of every page failing on its own. It is never cleared in the
// page's lifetime: granting the role takes effect on the next sign-in.
export const useAccessStore = create<AccessState>((set) => ({
  roleRequired: false,
  markRoleRequired: () => set({ roleRequired: true }),
}))
