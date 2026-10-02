/* Voxis Source-Available local Keycloak authentication. See the repository licensing files. */

import { createContext, useContext } from 'react'

export interface User {
  id: string
  email: string
  name: string
  username: string
}

export interface AuthContextType {
  isAuthenticated: boolean
  isLoading: boolean
  isLoggingOut: boolean
  user: User | null
  token: string | undefined
  error: string | null
  login: (redirectUri?: string) => Promise<void>
  logout: () => Promise<void>
  updateToken: (minValidity?: number) => Promise<boolean>
}

export const AuthContext = createContext<AuthContextType | null>(null)

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return context
}
