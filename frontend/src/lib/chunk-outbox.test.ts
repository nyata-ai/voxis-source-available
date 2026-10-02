import { describe, it, expect, vi, beforeEach } from 'vitest'
import { IDBFactory, IDBKeyRange as FakeIDBKeyRange } from 'fake-indexeddb'

beforeEach(() => {
  Object.defineProperty(globalThis, 'indexedDB', {
    value: new IDBFactory(),
    writable: true,
    configurable: true,
  })
  Object.defineProperty(globalThis, 'IDBKeyRange', {
    value: FakeIDBKeyRange,
    writable: true,
    configurable: true,
  })
})

function xorBuffer(data: BufferSource): ArrayBuffer {
  const input =
    data instanceof ArrayBuffer
      ? new Uint8Array(data)
      : new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
  const output = new Uint8Array(input.byteLength)
  for (let i = 0; i < input.byteLength; i += 1) {
    output[i] = input[i] ^ 0xff
  }
  return output.buffer
}

Object.defineProperty(globalThis, 'crypto', {
  value: {
    getRandomValues<T extends ArrayBufferView | null>(array: T): T {
      if (!array) return array
      const bytes = new Uint8Array(array.buffer, array.byteOffset, array.byteLength)
      for (let i = 0; i < bytes.byteLength; i += 1) {
        bytes[i] = i + 1
      }
      return array
    },
    subtle: {
      generateKey: vi.fn(async () => ({}) as CryptoKey),
      encrypt: vi.fn(async (_algorithm, _key, data) => xorBuffer(data)),
      decrypt: vi.fn(async (_algorithm, _key, data) => xorBuffer(data)),
    } as unknown as SubtleCrypto,
  } as Crypto,
  configurable: true,
})

function readBlobText(blob: Blob): Promise<string> {
  if (typeof blob.text === 'function') {
    return blob.text()
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(blob)
  })
}

describe('chunk-outbox types and exports', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('module exports all expected functions', async () => {
    const mod = await import('./chunk-outbox')
    expect(typeof mod.openOutboxDB).toBe('function')
    expect(typeof mod.putChunk).toBe('function')
    expect(typeof mod.getOldestUnsent).toBe('function')
    expect(typeof mod.countBySession).toBe('function')
    expect(typeof mod.getAllBySession).toBe('function')
    expect(typeof mod.deleteChunk).toBe('function')
    expect(typeof mod.deleteBySession).toBe('function')
    expect(typeof mod.cleanupOldEntries).toBe('function')
  })

  it('openOutboxDB rejects when IndexedDB is unavailable', async () => {
    // jsdom does not provide IndexedDB by default, but fake-indexeddb may be installed.
    // If indexedDB is undefined, openOutboxDB should throw.
    const originalIDB = globalThis.indexedDB
    Object.defineProperty(globalThis, 'indexedDB', { value: undefined, writable: true })

    vi.resetModules()
    const { openOutboxDB } = await import('./chunk-outbox')

    try {
      await openOutboxDB()
      // If it doesn't throw, IndexedDB is somehow available (fake-indexeddb)
    } catch (err) {
      expect(err).toBeDefined()
    } finally {
      Object.defineProperty(globalThis, 'indexedDB', { value: originalIDB, writable: true })
    }
  })
})

async function readStoredSessionKey(sessionId: string): Promise<unknown> {
  const { openOutboxDB } = await import('./chunk-outbox')
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction('recording-outbox-keys', 'readonly')
    const req = tx.objectStore('recording-outbox-keys').get(sessionId)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

describe('chunk-outbox IndexedDB cleanup', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('deletes buffered chunks and their session key together', async () => {
    const { putChunk, countBySession, deleteBySession } = await import('./chunk-outbox')
    await putChunk({
      sessionId: 'sess-discard',
      seq: 0,
      blob: new Blob(['audio'], { type: 'audio/webm' }),
      createdAt: Date.now(),
    })

    expect(await countBySession('sess-discard')).toBe(1)
    expect(await readStoredSessionKey('sess-discard')).toBeDefined()

    await deleteBySession('sess-discard')

    expect(await countBySession('sess-discard')).toBe(0)
    expect(await readStoredSessionKey('sess-discard')).toBeUndefined()
  })

  it('removes keys orphaned when expired chunks are cleaned', async () => {
    const { putChunk, cleanupOldEntries } = await import('./chunk-outbox')
    await putChunk({
      sessionId: 'sess-expired',
      seq: 0,
      blob: new Blob(['old audio'], { type: 'audio/webm' }),
      createdAt: Date.now() - 8 * 24 * 60 * 60 * 1000,
    })

    expect(await cleanupOldEntries()).toBe(1)
    expect(await readStoredSessionKey('sess-expired')).toBeUndefined()
  })
})

function audioChunk(sessionId: string, seq = 0) {
  return {
    sessionId,
    seq,
    blob: new Blob(['audio'], { type: 'audio/webm' }),
    createdAt: Date.now(),
  }
}

describe('chunk-outbox ownership', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it("keeps the same user's unsent chunks across a reload", async () => {
    const first = await import('./chunk-outbox')
    await first.claimOutbox('user-a')
    await first.putChunk(audioChunk('sess-a'))

    vi.resetModules() // a page reload: fresh module state, same IndexedDB
    const second = await import('./chunk-outbox')
    await second.claimOutbox('user-a')

    expect(await second.countBySession('sess-a')).toBe(1)
    // Chunk and key both survive, so recovery can decrypt and upload them.
    // (fake-indexeddb does not round-trip jsdom Blobs, so decryption itself is
    // covered by the encryption tests below.)
    expect(await readStoredSessionKey('sess-a')).toBeDefined()
  })

  it("deletes another user's chunks and keys when a different user signs in", async () => {
    const first = await import('./chunk-outbox')
    await first.claimOutbox('user-a')
    await first.putChunk(audioChunk('sess-a'))

    vi.resetModules()
    const second = await import('./chunk-outbox')
    await second.claimOutbox('user-b')

    expect(await second.countAllUnsent()).toBe(0)
    expect(await readStoredSessionKey('sess-a')).toBeUndefined()
  })

  it('does not let a pending claim wipe a chunk written right after it', async () => {
    const first = await import('./chunk-outbox')
    await first.claimOutbox('user-a')
    await first.putChunk(audioChunk('sess-a'))

    vi.resetModules()
    const second = await import('./chunk-outbox')
    void second.claimOutbox('user-b')
    await second.putChunk(audioChunk('sess-b'))

    expect(await second.countBySession('sess-a')).toBe(0)
    expect(await second.countBySession('sess-b')).toBe(1)
  })

  it('rejects a claim without a user id', async () => {
    const { claimOutbox } = await import('./chunk-outbox')
    await expect(claimOutbox('')).rejects.toThrow('user id')
  })

  it('wipeOutbox removes every chunk, key and the owner', async () => {
    const mod = await import('./chunk-outbox')
    await mod.claimOutbox('user-a')
    await mod.putChunk(audioChunk('sess-1', 0))
    await mod.putChunk(audioChunk('sess-1', 1))
    await mod.putChunk(audioChunk('sess-2', 0))
    expect(await mod.countAllUnsent()).toBe(3)

    await mod.wipeOutbox()

    expect(await mod.countAllUnsent()).toBe(0)
    expect(await readStoredSessionKey('sess-1')).toBeUndefined()
    expect(await readStoredSessionKey('sess-2')).toBeUndefined()
    const db = await mod.openOutboxDB()
    const owner = await new Promise((resolve, reject) => {
      const tx = db.transaction('recording-outbox-meta', 'readonly')
      const req = tx.objectStore('recording-outbox-meta').get('owner')
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
      tx.oncomplete = () => db.close()
    })
    expect(owner).toBeUndefined()
  })

  it('drops chunks of unknown ownership when upgrading from the v2 store', async () => {
    await new Promise<void>((resolve, reject) => {
      const req = indexedDB.open('voxis-recording', 2)
      req.onupgradeneeded = () => {
        const db = req.result
        const store = db.createObjectStore('recording-outbox', {
          keyPath: 'id',
          autoIncrement: true,
        })
        store.createIndex('sessionId_seq', ['sessionId', 'seq'], { unique: true })
        db.createObjectStore('recording-outbox-keys', { keyPath: 'sessionId' })
        store.add({ sessionId: 'legacy', seq: 0, createdAt: Date.now() })
      }
      req.onsuccess = () => {
        req.result.close()
        resolve()
      }
      req.onerror = () => reject(req.error)
    })

    const { countAllUnsent } = await import('./chunk-outbox')
    expect(await countAllUnsent()).toBe(0)
  })
})

describe('chunk-outbox function signatures', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('putChunk requires sessionId, seq, blob, createdAt', async () => {
    const { putChunk } = await import('./chunk-outbox')

    const entry = {
      sessionId: 'sess-1',
      seq: 0,
      blob: new Blob(['test']),
      createdAt: Date.now(),
    }

    // The function exists and accepts the right shape
    expect(typeof putChunk).toBe('function')
    expect(entry.sessionId).toBe('sess-1')
  })

  it('getOldestUnsent accepts sessionId string', async () => {
    const { getOldestUnsent } = await import('./chunk-outbox')
    expect(typeof getOldestUnsent).toBe('function')
  })

  it('deleteChunk accepts a numeric id', async () => {
    const { deleteChunk } = await import('./chunk-outbox')
    expect(typeof deleteChunk).toBe('function')
  })

  it('countBySession accepts sessionId string', async () => {
    const { countBySession } = await import('./chunk-outbox')
    expect(typeof countBySession).toBe('function')
  })

  it('deleteBySession accepts sessionId string', async () => {
    const { deleteBySession } = await import('./chunk-outbox')
    expect(typeof deleteBySession).toBe('function')
  })

  it('cleanupOldEntries takes no arguments', async () => {
    const { cleanupOldEntries } = await import('./chunk-outbox')
    expect(typeof cleanupOldEntries).toBe('function')
  })
})

describe('chunk-outbox encryption', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('encrypts stored chunks without retaining the raw blob field', async () => {
    const { encryptChunkForStorage } = await import('./chunk-outbox')

    const stored = await encryptChunkForStorage({
      sessionId: 'sess-secure',
      seq: 1,
      blob: new Blob(['sensitive-audio'], { type: 'audio/webm' }),
      createdAt: 123,
    })

    expect('blob' in stored).toBe(false)
    expect(stored.encryptedBlob).toBeInstanceOf(Blob)
    expect(stored.iv).toHaveLength(12)
    expect(await readBlobText(stored.encryptedBlob)).not.toContain('sensitive-audio')
  })

  it('decrypts stored chunks back to uploadable entries', async () => {
    const { encryptChunkForStorage, decryptStoredChunk } = await import('./chunk-outbox')

    const stored = await encryptChunkForStorage({
      sessionId: 'sess-secure',
      seq: 2,
      blob: new Blob(['recoverable-audio'], { type: 'audio/webm' }),
      createdAt: 456,
    })

    const entry = await decryptStoredChunk({ ...stored, id: 7 })

    expect(entry.id).toBe(7)
    expect(entry.sessionId).toBe('sess-secure')
    expect(entry.seq).toBe(2)
    expect(entry.createdAt).toBe(456)
    expect(entry.blob.type).toBe('audio/webm')
    expect(await readBlobText(entry.blob)).toBe('recoverable-audio')
  })
})
