import { create } from 'zustand'

export const ALLOWED_AUDIO_TYPES = new Set([
  'audio/mpeg',
  'audio/wav',
  'audio/ogg',
  'audio/flac',
  'audio/mp4',
  'audio/webm',
  'audio/x-m4a',
  'audio/aac',
  'audio/x-wav',
  'audio/opus',
])

export const MAX_FILE_SIZE = 1024 * 1024 * 1024 // 1 GB

const MIME_ALIASES: Record<string, string> = {
  'audio/vnd.dlna.adts': 'audio/aac',
  'audio/x-aac': 'audio/aac',
  'audio/x-flac': 'audio/flac',
  'audio/mp3': 'audio/mpeg',
  'audio/x-mpeg': 'audio/mpeg',
  'audio/wave': 'audio/wav',
  'audio/vnd.wave': 'audio/wav',
  'audio/x-opus+ogg': 'audio/opus',
}

const EXTENSION_TO_MIME: Record<string, string> = {
  mp3: 'audio/mpeg',
  wav: 'audio/wav',
  ogg: 'audio/ogg',
  flac: 'audio/flac',
  m4a: 'audio/mp4',
  webm: 'audio/webm',
  aac: 'audio/aac',
  opus: 'audio/opus',
}

// canonicalAudioType accepts browser and operating-system aliases before the
// upload is constructed. This keeps the multipart type consistent with the
// server's extension and signature checks.
export function canonicalAudioType(file: File): string | null {
  const declared = file.type.toLowerCase()
  const aliased = MIME_ALIASES[declared] ?? declared
  if (ALLOWED_AUDIO_TYPES.has(aliased)) return aliased

  const dot = file.name.lastIndexOf('.')
  if (dot < 0) return null
  return EXTENSION_TO_MIME[file.name.slice(dot + 1).toLowerCase()] ?? null
}

export interface UploadItem {
  id: string
  file: File
  status: 'queued' | 'uploading' | 'processing' | 'complete' | 'error'
  progress: number
  mediaId?: string
  error?: string
}

export type UploadRejectionReason = 'unsupportedType' | 'tooLarge' | 'storageQuota'

export interface RejectedFile {
  file: File
  reason: UploadRejectionReason
}

interface UploadState {
  queue: UploadItem[]
  rejected: RejectedFile[]
  addFiles: (files: File[], remainingStorageBytes?: number) => RejectedFile[]
  setProgress: (id: string, progress: number) => void
  setProcessing: (id: string) => void
  setComplete: (id: string, mediaId: string) => void
  setError: (id: string, error: string) => void
  setUploading: (id: string) => void
  /** Returns a row to the queue (progress 0) while it waits for server capacity. */
  setQueued: (id: string) => void
  removeItem: (id: string) => void
  reset: () => void
}

let nextId = 0

function generateId(): string {
  return `upload-${Date.now()}-${nextId++}`
}

export const useUploadStore = create<UploadState>((set) => ({
  queue: [],
  rejected: [],

  addFiles: (files: File[], remainingStorageBytes?: number) => {
    const rejected: RejectedFile[] = []
    const accepted: UploadItem[] = []
    let remaining: number | null = null
    if (
      typeof remainingStorageBytes === 'number' &&
      Number.isSafeInteger(remainingStorageBytes) &&
      remainingStorageBytes >= 0
    ) {
      remaining = remainingStorageBytes
    }

    for (const original of files) {
      const canonical = canonicalAudioType(original)
      if (canonical === null) {
        rejected.push({ file: original, reason: 'unsupportedType' })
        continue
      }

      if (original.size > MAX_FILE_SIZE) {
        rejected.push({ file: original, reason: 'tooLarge' })
        continue
      }

      if (remaining !== null && original.size > remaining) {
        rejected.push({ file: original, reason: 'storageQuota' })
        continue
      }

      const file = canonical === original.type
        ? original
        : new File([original], original.name, {
            type: canonical,
            lastModified: original.lastModified,
          })

      accepted.push({
        id: generateId(),
        file,
        status: 'queued',
        progress: 0,
      })
      if (remaining !== null) remaining -= original.size
    }

    if (accepted.length > 0) {
      set((state) => ({ queue: [...state.queue, ...accepted] }))
    }

    set({ rejected })

    return rejected
  },

  setProgress: (id, progress) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id ? { ...item, progress, status: 'uploading' as const } : item
      ),
    }))
  },

  setProcessing: (id) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id ? { ...item, status: 'processing' as const, progress: 100 } : item
      ),
    }))
  },

  setComplete: (id, mediaId) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id
          ? { ...item, status: 'complete' as const, progress: 100, mediaId }
          : item
      ),
    }))
  },

  setError: (id, error) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id ? { ...item, status: 'error' as const, error } : item
      ),
    }))
  },

  setUploading: (id) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id ? { ...item, status: 'uploading' as const } : item
      ),
    }))
  },

  setQueued: (id) => {
    set((state) => ({
      queue: state.queue.map((item) =>
        item.id === id ? { ...item, status: 'queued' as const, progress: 0 } : item
      ),
    }))
  },

  removeItem: (id) => {
    set((state) => ({
      queue: state.queue.filter((item) => item.id !== id),
    }))
  },

  reset: () => {
    set({ queue: [], rejected: [] })
  },
}))
