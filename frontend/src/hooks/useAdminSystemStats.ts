import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/lib/api-client'

export interface SystemCPU { load_1: number; load_5: number; load_15: number; cores: number }
export interface SystemMemory { used_bytes: number; available_bytes: number; total_bytes: number }
export interface SystemUptime { host_seconds: number; process_seconds: number }
export interface SystemHost {
  available: boolean; reason?: string; ip: string; external_ip?: string
  cpu: SystemCPU; memory: SystemMemory; uptime: SystemUptime
}
export interface SystemBuild { available: boolean; reason?: string; version: string; commit: string; build_time: string }
export interface SystemDisk {
  available: boolean; reason?: string; mount: string
  used_bytes: number; free_bytes: number; total_bytes: number
}
export interface SystemGCS { bucket: string; has_data: boolean; total_bytes: number; object_count: number; as_of: string }
export interface SystemStorage { available: boolean; reason?: string; backend: string; gcs?: SystemGCS }
export interface SystemDBPool { acquired: number; idle: number; max: number; total: number }
export interface SystemDatabase { available: boolean; reason?: string; size_bytes: number; pool: SystemDBPool }
export interface SystemUsers {
  available: boolean; reason?: string; total: number; organizations: number; new_7d: number; new_30d: number
}
export interface SystemDependency { name: string; status: 'up' | 'down'; latency_ms: number; reason?: string }

export interface AdminSystemStats {
  generated_at: string
  host: SystemHost
  build: SystemBuild
  disk: SystemDisk
  storage: SystemStorage
  database: SystemDatabase
  users: SystemUsers
  dependencies: SystemDependency[]
}

export const adminSystemKeys = {
  all: ['admin', 'system'] as const,
  stats: () => [...adminSystemKeys.all, 'stats'] as const,
}

const unavailable = (reason = 'unavailable') => ({ available: false, reason })

export function normalizeAdminSystemStats(input?: Partial<AdminSystemStats> | null): AdminSystemStats {
  const s = input ?? {}
  return {
    generated_at: s.generated_at ?? '',
    host: s.host ?? { ...unavailable(), ip: 'unknown',
      cpu: { load_1: 0, load_5: 0, load_15: 0, cores: 0 },
      memory: { used_bytes: 0, available_bytes: 0, total_bytes: 0 },
      uptime: { host_seconds: 0, process_seconds: 0 } },
    build: s.build ?? { ...unavailable(), version: '', commit: '', build_time: '' },
    disk: s.disk ?? { ...unavailable(), mount: '/', used_bytes: 0, free_bytes: 0, total_bytes: 0 },
    storage: s.storage ?? { ...unavailable(), backend: 'unknown' },
    database: s.database ?? { ...unavailable(), size_bytes: 0, pool: { acquired: 0, idle: 0, max: 0, total: 0 } },
    users: s.users ?? { ...unavailable(), total: 0, organizations: 0, new_7d: 0, new_30d: 0 },
    dependencies: s.dependencies ?? [],
  }
}

export function useAdminSystemStats() {
  return useQuery({
    queryKey: adminSystemKeys.stats(),
    queryFn: async () =>
      normalizeAdminSystemStats(await apiClient.get<AdminSystemStats>('/admin/system/stats')),
    staleTime: 30_000,
  })
}
