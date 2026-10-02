import { describe, it, expect, beforeEach } from 'vitest'
import { canonicalAudioType, useUploadStore, ALLOWED_AUDIO_TYPES, MAX_FILE_SIZE } from './upload'

function createFile(name: string, type: string, size: number): File {
  // For large sizes (>10MB), use Object.defineProperty to fake size
  // instead of allocating a real buffer (which would OOM or timeout).
  if (size > 10 * 1024 * 1024) {
    const file = new File([], name, { type })
    Object.defineProperty(file, 'size', { value: size })
    return file
  }
  const buffer = new ArrayBuffer(size)
  return new File([buffer], name, { type })
}

describe('canonicalAudioType', () => {
  it.each([
    ['clip.aac', 'audio/vnd.dlna.adts', 'audio/aac'],
    ['clip.aac', 'audio/x-aac', 'audio/aac'],
    ['clip.flac', 'audio/x-flac', 'audio/flac'],
    ['clip.mp3', 'audio/mp3', 'audio/mpeg'],
    ['clip.mp3', 'audio/x-mpeg', 'audio/mpeg'],
    ['clip.wav', 'audio/wave', 'audio/wav'],
    ['clip.wav', 'audio/vnd.wave', 'audio/wav'],
    ['clip.opus', 'audio/x-opus+ogg', 'audio/opus'],
  ])('maps %s (%s) to %s', (name, type, expected) => {
    expect(canonicalAudioType(createFile(name, type, 16))).toBe(expected)
  })

  it('falls back to the extension for a generic type', () => {
    expect(canonicalAudioType(createFile('clip.M4A', 'application/octet-stream', 16))).toBe('audio/mp4')
  })

  it('returns null when neither type nor extension is audio', () => {
    expect(canonicalAudioType(createFile('archive', 'application/zip', 16))).toBeNull()
  })
})

describe('useUploadStore', () => {
  beforeEach(() => {
    useUploadStore.getState().reset()
  })

  describe('addFiles', () => {
    it('should add valid audio files to queue', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(0)
      const queue = useUploadStore.getState().queue
      expect(queue).toHaveLength(1)
      expect(queue[0].file).toBe(file)
      expect(queue[0].status).toBe('queued')
      expect(queue[0].progress).toBe(0)
    })

    it('should add multiple valid files', () => {
      const files = [
        createFile('a.mp3', 'audio/mpeg', 1024),
        createFile('b.wav', 'audio/wav', 2048),
        createFile('c.ogg', 'audio/ogg', 512),
      ]

      const rejected = useUploadStore.getState().addFiles(files)

      expect(rejected).toHaveLength(0)
      expect(useUploadStore.getState().queue).toHaveLength(3)
    })

    it('should reject non-audio files', () => {
      const file = createFile('document.pdf', 'application/pdf', 1024)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(1)
      expect(rejected[0].file).toBe(file)
      expect(rejected[0].reason).toBe('unsupportedType')
      expect(useUploadStore.getState().queue).toHaveLength(0)
    })

    it('should reject files exceeding max size', () => {
      const file = createFile('huge.mp3', 'audio/mpeg', MAX_FILE_SIZE + 1)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(1)
      expect(rejected[0].file).toBe(file)
      expect(rejected[0].reason).toBe('tooLarge')
      expect(useUploadStore.getState().queue).toHaveLength(0)
    })

    it('should accept files at exactly max size', () => {
      const file = createFile('max.mp3', 'audio/mpeg', MAX_FILE_SIZE)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(0)
      expect(useUploadStore.getState().queue).toHaveLength(1)
    })

    it('should accept and reject in the same batch', () => {
      const files = [
        createFile('good.mp3', 'audio/mpeg', 1024),
        createFile('bad.pdf', 'application/pdf', 1024),
        createFile('good.wav', 'audio/wav', 2048),
      ]

      const rejected = useUploadStore.getState().addFiles(files)

      expect(rejected).toHaveLength(1)
      expect(rejected[0].file.name).toBe('bad.pdf')
      expect(useUploadStore.getState().queue).toHaveLength(2)
    })

    it('rejects files that would exceed the remaining quota in the same drop', () => {
      const files = [
        createFile('first.mp3', 'audio/mpeg', 60),
        createFile('second.mp3', 'audio/mpeg', 50),
      ]

      const rejected = useUploadStore.getState().addFiles(files, 100)

      expect(useUploadStore.getState().queue.map((item) => item.file.name)).toEqual(['first.mp3'])
      expect(rejected).toEqual([{ file: files[1], reason: 'storageQuota' }])
    })

    it('accepts .aac reported by Windows and sends the canonical MIME type', () => {
      const file = createFile('interview.aac', 'audio/vnd.dlna.adts', 1024)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(0)
      expect(useUploadStore.getState().queue[0].file.type).toBe('audio/aac')
      expect(useUploadStore.getState().queue[0].file.name).toBe('interview.aac')
    })

    it('uses an audio extension when the browser provides no MIME type', () => {
      const file = createFile('interview.aac', '', 1024)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toHaveLength(0)
      expect(useUploadStore.getState().queue[0].file.type).toBe('audio/aac')
    })

    it('rejects a non-audio extension when the browser provides no MIME type', () => {
      const file = createFile('notes.txt', '', 1024)

      const rejected = useUploadStore.getState().addFiles([file])

      expect(rejected).toEqual([{ file, reason: 'unsupportedType' }])
      expect(useUploadStore.getState().queue).toHaveLength(0)
    })

    it('should generate unique ids for each item', () => {
      const files = [
        createFile('a.mp3', 'audio/mpeg', 1024),
        createFile('b.mp3', 'audio/mpeg', 1024),
      ]

      useUploadStore.getState().addFiles(files)

      const queue = useUploadStore.getState().queue
      expect(queue[0].id).not.toBe(queue[1].id)
    })
  })

  describe('setProgress', () => {
    it('should update progress and set status to uploading', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)
      useUploadStore.getState().addFiles([file])
      const id = useUploadStore.getState().queue[0].id

      useUploadStore.getState().setProgress(id, 50)

      const item = useUploadStore.getState().queue[0]
      expect(item.progress).toBe(50)
      expect(item.status).toBe('uploading')
    })
  })

  describe('setUploading', () => {
    it('should set status to uploading', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)
      useUploadStore.getState().addFiles([file])
      const id = useUploadStore.getState().queue[0].id

      useUploadStore.getState().setUploading(id)

      expect(useUploadStore.getState().queue[0].status).toBe('uploading')
    })
  })

  describe('setProcessing', () => {
    it('should set status to processing and progress to 100', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)
      useUploadStore.getState().addFiles([file])
      const id = useUploadStore.getState().queue[0].id
      useUploadStore.getState().setUploading(id)
      useUploadStore.getState().setProgress(id, 99)

      useUploadStore.getState().setProcessing(id)

      const item = useUploadStore.getState().queue[0]
      expect(item.status).toBe('processing')
      expect(item.progress).toBe(100)
    })
  })

  describe('setComplete', () => {
    it('should mark item as complete with mediaId', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)
      useUploadStore.getState().addFiles([file])
      const id = useUploadStore.getState().queue[0].id

      useUploadStore.getState().setComplete(id, 'media-123')

      const item = useUploadStore.getState().queue[0]
      expect(item.status).toBe('complete')
      expect(item.progress).toBe(100)
      expect(item.mediaId).toBe('media-123')
    })
  })

  describe('setError', () => {
    it('should mark item as error with message', () => {
      const file = createFile('recording.mp3', 'audio/mpeg', 1024)
      useUploadStore.getState().addFiles([file])
      const id = useUploadStore.getState().queue[0].id

      useUploadStore.getState().setError(id, 'Upload failed')

      const item = useUploadStore.getState().queue[0]
      expect(item.status).toBe('error')
      expect(item.error).toBe('Upload failed')
    })
  })

  describe('removeItem', () => {
    it('should remove item from queue', () => {
      const files = [
        createFile('a.mp3', 'audio/mpeg', 1024),
        createFile('b.mp3', 'audio/mpeg', 1024),
      ]
      useUploadStore.getState().addFiles(files)
      const id = useUploadStore.getState().queue[0].id

      useUploadStore.getState().removeItem(id)

      expect(useUploadStore.getState().queue).toHaveLength(1)
      expect(useUploadStore.getState().queue[0].file.name).toBe('b.mp3')
    })
  })

  describe('reset', () => {
    it('should clear the queue', () => {
      const files = [
        createFile('a.mp3', 'audio/mpeg', 1024),
        createFile('b.mp3', 'audio/mpeg', 1024),
      ]
      useUploadStore.getState().addFiles(files)
      expect(useUploadStore.getState().queue).toHaveLength(2)

      useUploadStore.getState().reset()

      expect(useUploadStore.getState().queue).toHaveLength(0)
    })
  })

  describe('ALLOWED_AUDIO_TYPES', () => {
    it('should contain expected audio MIME types', () => {
      expect(ALLOWED_AUDIO_TYPES.has('audio/mpeg')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/wav')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/ogg')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/flac')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/mp4')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/webm')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/x-m4a')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/aac')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/x-wav')).toBe(true)
      expect(ALLOWED_AUDIO_TYPES.has('audio/opus')).toBe(true)
    })

    it('should not contain non-audio types', () => {
      expect(ALLOWED_AUDIO_TYPES.has('application/pdf')).toBe(false)
      expect(ALLOWED_AUDIO_TYPES.has('video/mp4')).toBe(false)
    })
  })

  describe('MAX_FILE_SIZE', () => {
    it('should be 1GB', () => {
      expect(MAX_FILE_SIZE).toBe(1024 * 1024 * 1024)
    })
  })
})
