import type {
  OutboxEntry,
  StoredOutboxEntry,
  StoredOutboxKey,
  StoredOutboxOwner,
} from '@/types/recording'

const DB_NAME = 'voxis-recording'
const DB_VERSION = 3
const STORE_NAME = 'recording-outbox'
const KEY_STORE_NAME = 'recording-outbox-keys'
const META_STORE_NAME = 'recording-outbox-meta'
const OWNER_META_NAME = 'owner'
const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000
const AES_GCM_IV_BYTES = 12

const sessionKeys = new Map<string, Promise<CryptoKey>>()

async function getSessionKey(sessionId: string): Promise<CryptoKey> {
  let key = sessionKeys.get(sessionId)
  if (!key) {
    key = loadOrCreateSessionKey(sessionId)
    sessionKeys.set(sessionId, key)
  }
  return key
}

async function loadOrCreateSessionKey(sessionId: string): Promise<CryptoKey> {
  const candidateKey = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, [
    'encrypt',
    'decrypt',
  ])
  let db: IDBDatabase
  try {
    db = await openOutboxDB()
  } catch {
    return candidateKey
  }

  return new Promise((resolve, reject) => {
    const tx = db.transaction(KEY_STORE_NAME, 'readwrite')
    const store = tx.objectStore(KEY_STORE_NAME)
    const getReq = store.get(sessionId)

    getReq.onsuccess = () => {
      const stored = getReq.result as StoredOutboxKey | undefined
      if (stored?.key) {
        resolve(stored.key)
        return
      }

      const putReq = store.put({ sessionId, key: candidateKey } satisfies StoredOutboxKey)
      putReq.onsuccess = () => resolve(candidateKey)
      putReq.onerror = () => reject(putReq.error)
    }
    getReq.onerror = () => reject(getReq.error)
    tx.oncomplete = () => db.close()
    tx.onerror = () => {
      db.close()
      reject(tx.error)
    }
  })
}

function blobToArrayBuffer(blob: Blob): Promise<ArrayBuffer> {
  if (typeof blob.arrayBuffer === 'function') {
    return blob.arrayBuffer()
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as ArrayBuffer)
    reader.onerror = () => reject(reader.error)
    reader.readAsArrayBuffer(blob)
  })
}

export async function encryptChunkForStorage(
  entry: Omit<OutboxEntry, 'id'>
): Promise<Omit<StoredOutboxEntry, 'id'>> {
  const key = await getSessionKey(entry.sessionId)
  const iv = crypto.getRandomValues(new Uint8Array(AES_GCM_IV_BYTES))
  const plaintext = await blobToArrayBuffer(entry.blob)
  const ciphertext = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv },
    key,
    new Uint8Array(plaintext)
  )

  return {
    sessionId: entry.sessionId,
    seq: entry.seq,
    encryptedBlob: new Blob([ciphertext], { type: 'application/octet-stream' }),
    iv: Array.from(iv),
    mimeType: entry.blob.type,
    createdAt: entry.createdAt,
  }
}

export async function decryptStoredChunk(entry: StoredOutboxEntry): Promise<OutboxEntry> {
  const key = await getSessionKey(entry.sessionId)
  const iv = new Uint8Array(entry.iv)
  const ciphertext = await blobToArrayBuffer(entry.encryptedBlob)
  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv },
    key,
    new Uint8Array(ciphertext)
  )

  return {
    id: entry.id,
    sessionId: entry.sessionId,
    seq: entry.seq,
    blob: new Blob([plaintext], { type: entry.mimeType }),
    createdAt: entry.createdAt,
  }
}

/**
 * Open (or create) the IndexedDB database for the recording outbox.
 * Returns a promise that resolves to the IDBDatabase instance.
 */
export function openOutboxDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)

    request.onupgradeneeded = (event) => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, {
          keyPath: 'id',
          autoIncrement: true,
        })
        store.createIndex('sessionId_seq', ['sessionId', 'seq'], { unique: true })
      }
      if (!db.objectStoreNames.contains(KEY_STORE_NAME)) {
        db.createObjectStore(KEY_STORE_NAME, { keyPath: 'sessionId' })
      }
      if (!db.objectStoreNames.contains(META_STORE_NAME)) {
        db.createObjectStore(META_STORE_NAME, { keyPath: 'name' })
      }
      if (event.oldVersion > 0 && event.oldVersion < 3) {
        // v1 stored raw audio blobs; v2 did not record which account made the
        // recording. Drop both rather than hand audio of unknown ownership to
        // whoever signs in next on this browser profile.
        request.transaction?.objectStore(STORE_NAME).clear()
        request.transaction?.objectStore(KEY_STORE_NAME).clear()
      }
    }

    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

// Latest ownership claim. Outbox reads and writes wait for it, so a chunk is
// never written before (and then wiped by) a claim that is still running, and
// a new user never reads chunks the claim is about to remove.
let ownerClaim: Promise<void> = Promise.resolve()

function afterOwnerClaim(): Promise<void> {
  return ownerClaim.catch(() => {})
}

/**
 * Bind the outbox to the signed-in user (the Keycloak `sub`). If it holds
 * chunks recorded by a different user, they and their keys are deleted first:
 * on a shared browser profile the next person must not be able to decrypt
 * someone else's unsent audio. The same user keeps their chunks, so recovery
 * after a crash or reload still works. Call once per sign-in.
 */
export function claimOutbox(userId: string): Promise<void> {
  if (!userId) {
    return Promise.reject(new Error('claimOutbox requires a user id'))
  }
  const claim = afterOwnerClaim().then(() => applyOwnerClaim(userId))
  ownerClaim = claim
  return claim
}

async function applyOwnerClaim(userId: string): Promise<void> {
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_NAME, KEY_STORE_NAME, META_STORE_NAME], 'readwrite')
    const meta = tx.objectStore(META_STORE_NAME)
    const getReq = meta.get(OWNER_META_NAME)
    let wiped = false

    getReq.onsuccess = () => {
      const owner = getReq.result as StoredOutboxOwner | undefined
      if (owner?.userId === userId) return
      if (owner) {
        tx.objectStore(STORE_NAME).clear()
        tx.objectStore(KEY_STORE_NAME).clear()
        wiped = true
      }
      meta.put({ name: OWNER_META_NAME, userId } satisfies StoredOutboxOwner)
    }
    tx.oncomplete = () => {
      db.close()
      if (wiped) sessionKeys.clear()
      resolve()
    }
    tx.onerror = () => {
      db.close()
      reject(tx.error)
    }
  })
}

/** Number of buffered chunks across every session (what signing out would lose). */
export async function countAllUnsent(): Promise<number> {
  await afterOwnerClaim()
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const req = tx.objectStore(STORE_NAME).count()
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/**
 * Delete every buffered chunk, every key and the owner record. Runs on sign-out
 * so nothing decryptable is left behind for the next user of this browser.
 */
export async function wipeOutbox(): Promise<void> {
  await afterOwnerClaim()
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_NAME, KEY_STORE_NAME, META_STORE_NAME], 'readwrite')
    tx.objectStore(STORE_NAME).clear()
    tx.objectStore(KEY_STORE_NAME).clear()
    tx.objectStore(META_STORE_NAME).clear()
    tx.oncomplete = () => {
      db.close()
      sessionKeys.clear()
      resolve()
    }
    tx.onerror = () => {
      db.close()
      reject(tx.error)
    }
  })
}

/** Insert a chunk entry into the outbox. */
export async function putChunk(entry: Omit<OutboxEntry, 'id'>): Promise<number> {
  await afterOwnerClaim()
  const storedEntry = await encryptChunkForStorage(entry)
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.add(storedEntry)
    req.onsuccess = () => resolve(req.result as number)
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/**
 * Get the oldest unsent entry for a given session.
 * Reads all entries for the session and returns the one with the smallest seq.
 */
export async function getOldestUnsent(sessionId: string): Promise<OutboxEntry | null> {
  await afterOwnerClaim()
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const index = store.index('sessionId_seq')

    // Use a key range that starts at [sessionId, lowest possible] and ends at [sessionId, highest]
    const range = IDBKeyRange.bound([sessionId, -Infinity], [sessionId, Infinity])
    const req = index.openCursor(range)

    req.onsuccess = () => {
      const cursor = req.result
      if (cursor) {
        decryptStoredChunk(cursor.value as StoredOutboxEntry)
          .then(resolve)
          .catch(reject)
      } else {
        resolve(null)
      }
    }
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/** Count unsent entries for a given session. */
export async function countBySession(sessionId: string): Promise<number> {
  await afterOwnerClaim()
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const index = store.index('sessionId_seq')
    const range = IDBKeyRange.bound([sessionId, -Infinity], [sessionId, Infinity])
    const req = index.count(range)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/** Get all entries for a given session, ordered by seq. */
export async function getAllBySession(sessionId: string): Promise<OutboxEntry[]> {
  await afterOwnerClaim()
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const index = store.index('sessionId_seq')
    const range = IDBKeyRange.bound([sessionId, -Infinity], [sessionId, Infinity])
    const req = index.getAll(range)
    req.onsuccess = () => {
      Promise.all((req.result as StoredOutboxEntry[]).map(decryptStoredChunk))
        .then(resolve)
        .catch(reject)
    }
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/** Delete an outbox entry by its auto-increment key. */
export async function deleteChunk(id: number): Promise<void> {
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.delete(id)
    req.onsuccess = () => resolve()
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => db.close()
  })
}

/** Delete all outbox entries for a given session. */
export async function deleteBySession(sessionId: string): Promise<void> {
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_NAME, KEY_STORE_NAME], 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const index = store.index('sessionId_seq')
    const range = IDBKeyRange.bound([sessionId, -Infinity], [sessionId, Infinity])
    const cursor = index.openCursor(range)

    cursor.onsuccess = () => {
      const entry = cursor.result
      if (!entry) return
      entry.delete()
      entry.continue()
    }
    cursor.onerror = () => reject(cursor.error)
    tx.objectStore(KEY_STORE_NAME).delete(sessionId)
    tx.oncomplete = () => {
      db.close()
      sessionKeys.delete(sessionId)
      resolve()
    }
    tx.onerror = () => {
      db.close()
      reject(tx.error)
    }
  })
}

/**
 * Delete outbox entries older than 7 days (orphan safety net).
 * Call on app startup.
 */
export async function cleanupOldEntries(): Promise<number> {
  const db = await openOutboxDB()
  const cutoff = Date.now() - SEVEN_DAYS_MS

  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.openCursor()
    let deleted = 0

    req.onsuccess = () => {
      const cursor = req.result
      if (cursor) {
        const entry = cursor.value as StoredOutboxEntry
        if (entry.createdAt < cutoff) {
          cursor.delete()
          deleted++
        }
        cursor.continue()
      }
    }
    req.onerror = () => reject(req.error)
    tx.oncomplete = () => {
      db.close()
      cleanupOrphanedSessionKeys().then(() => resolve(deleted)).catch(reject)
    }
  })
}

/** Remove keys whose session no longer has any buffered chunks. */
async function cleanupOrphanedSessionKeys(): Promise<void> {
  const db = await openOutboxDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction([STORE_NAME, KEY_STORE_NAME], 'readwrite')
    const entries = tx.objectStore(STORE_NAME)
    const keys = tx.objectStore(KEY_STORE_NAME)
    const keyCursor = keys.openCursor()

    keyCursor.onsuccess = () => {
      const cursor = keyCursor.result
      if (!cursor) return

      const sessionId = (cursor.value as StoredOutboxKey).sessionId
      const range = IDBKeyRange.bound([sessionId, -Infinity], [sessionId, Infinity])
      const count = entries.index('sessionId_seq').count(range)
      count.onsuccess = () => {
        if (count.result === 0) {
          cursor.delete()
          sessionKeys.delete(sessionId)
        }
        cursor.continue()
      }
      count.onerror = () => reject(count.error)
    }
    keyCursor.onerror = () => reject(keyCursor.error)
    tx.oncomplete = () => {
      db.close()
      resolve()
    }
    tx.onerror = () => {
      db.close()
      reject(tx.error)
    }
  })
}
