import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * Merge class names with Tailwind CSS conflict resolution
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Format duration in seconds to mm:ss or hh:mm:ss format
 */
export function formatDuration(seconds: number): string {
  if (seconds < 0) return '00:00'

  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  const pad = (n: number) => n.toString().padStart(2, '0')

  if (hours > 0) {
    return `${pad(hours)}:${pad(minutes)}:${pad(secs)}`
  }
  return `${pad(minutes)}:${pad(secs)}`
}

/**
 * Format bytes to human-readable format
 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes <= 0) return '0 B'

  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`
}

/**
 * Format an Indonesian Rupiah amount. IDR has no minor units in practice, so
 * fractions are always dropped. Always rendered in the id-ID convention
 * ("Rp 85.000") regardless of UI language — it is a real-world price, not a
 * localized number.
 */
export function formatIDR(amount: number): string {
  const value = Number.isFinite(amount) ? amount : 0
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(value)
}

/** Format duration in seconds to a human-friendly string (e.g. "1 min 23 sec"). */
export function formatDurationHuman(seconds: number): string {
  if (seconds <= 0) return '0 sec'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)

  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m} min ${s > 0 ? `${s} sec` : ''}`
  return `${s} sec`
}
