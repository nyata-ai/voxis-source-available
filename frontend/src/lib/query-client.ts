import { QueryClient } from '@tanstack/react-query'
import { ApiError } from './api-client'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Don't retry on 4xx errors (client errors)
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
          return false
        }
        return failureCount < 3
      },
      // Stale time: 1 minute
      staleTime: 60 * 1000,
      // Keep unused data for 5 minutes
      gcTime: 5 * 60 * 1000,
      // Refetch on window focus (good for long sessions)
      refetchOnWindowFocus: true,
      // Don't refetch on reconnect by default
      refetchOnReconnect: false,
    },
    mutations: {
      // Don't retry mutations
      retry: false,
    },
  },
})
