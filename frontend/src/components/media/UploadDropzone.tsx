import { useCallback, useRef, type MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { useDropzone } from 'react-dropzone'
import { Upload, X, CheckCircle, AlertCircle, FileAudio, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useUploadStore, type UploadItem } from '@/stores/upload'
import { dismissCapture, useCaptureStore } from '@/stores/capture'
import { useUploadMedia } from '@/hooks/useMedia'
import { useUsageStats } from '@/hooks/useUsageStats'
import { cn, formatBytes } from '@/lib/utils'
import {
  getUploadErrorMessage,
  isUploadCapacityError,
  uploadRetryDelayMs,
  uploadScheduler,
} from './upload-utils'
import { StorageQuotaNotice } from './StorageQuotaNotice'

const ACCEPTED_TYPES = {
  'audio/*': ['.mp3', '.wav', '.ogg', '.flac', '.m4a', '.webm', '.aac', '.opus'],
}

const MAX_SIZE = 1024 * 1024 * 1024 // 1 GB

// A browser sends at most this many files at once; the rest wait as "queued"
// and start as others settle. Keeps one multi-file drop from claiming every
// server upload slot, and bounds how much bandwidth a same-origin deployment
// (where nginx buffers the whole body before the API can refuse it) spends
// on refusals.
const CLIENT_UPLOAD_PARALLELISM = 2

// The server admits a bounded number of simultaneous uploads and answers 503
// with Retry-After when it is full. A file waits and retries by itself rather
// than failing; this caps the wait (20 × ~15 s ≈ 5 minutes) so a server that
// never frees up still ends in a visible error.
const UPLOAD_CAPACITY_RETRIES = 20

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function UploadDropzone() {
  const { t } = useTranslation('media')
  const queue = useUploadStore((s) => s.queue)
  const rejected = useUploadStore((s) => s.rejected)
  const addFiles = useUploadStore((s) => s.addFiles)
  const removeItem = useUploadStore((s) => s.removeItem)
  const setUploading = useUploadStore((s) => s.setUploading)
  const setProgress = useUploadStore((s) => s.setProgress)
  const setProcessing = useUploadStore((s) => s.setProcessing)
  const setComplete = useUploadStore((s) => s.setComplete)
  const setError = useUploadStore((s) => s.setError)
  const setQueued = useUploadStore((s) => s.setQueued)
  const { mutateAsync } = useUploadMedia()
  const { data: usageStats, refetch: refetchUsage } = useUsageStats()
  const navigate = useNavigate()
  // Drops being admitted right now: incremented on entry to onDrop, released
  // once addFiles has run. onDrop awaits a quota refetch before the queue
  // learns about the new files, so during that window `queue` understates the
  // session — auto-advance must not fire off a stale count.
  const pendingDrops = useRef(0)

  // Sends one file, waiting out "server full" answers. Resolves to null when
  // the row was removed while it waited (the user cleared it), so nothing is
  // uploaded invisibly. Any other failure is thrown to the caller unchanged.
  const uploadWithCapacityRetry = useCallback(
    async (item: UploadItem) => {
      const formData = new FormData()
      formData.append('file', item.file)
      const stillListed = () => useUploadStore.getState().queue.some((q) => q.id === item.id)
      for (let attempt = 0; ; attempt++) {
        if (!stillListed()) return null
        setUploading(item.id)
        try {
          return await mutateAsync({
            formData,
            onProgress: (pct) => {
              setProgress(item.id, pct)
              if (pct === 100) {
                setProcessing(item.id)
              }
            },
          })
        } catch (err) {
          if (!isUploadCapacityError(err) || attempt >= UPLOAD_CAPACITY_RETRIES) {
            throw err
          }
          setQueued(item.id)
          await sleep(uploadRetryDelayMs(err))
        }
      }
    },
    [mutateAsync, setProcessing, setProgress, setQueued, setUploading]
  )

  const uploadFile = useCallback(
    async (item: UploadItem) => {
      try {
        const result = await uploadWithCapacityRetry(item)
        if (!result) return
        setComplete(item.id, result.id)
        // Single-file convenience: hand the user straight to the file page so
        // they can start transcribing. Only when this is the sole queued item,
        // nothing was rejected, and the dialog is still open on this tab — a
        // multi-file batch keeps its per-row links, rejection notices must
        // stay readable, and an upload that outlived the dialog (or a user
        // who moved to another tab) must not have navigation yanked from
        // under them.
        const { queue, rejected } = useUploadStore.getState()
        const capture = useCaptureStore.getState()
        if (
          capture.open &&
          capture.tab === 'upload' &&
          pendingDrops.current === 0 &&
          rejected.length === 0 &&
          queue.length === 1 &&
          queue[0].id === item.id
        ) {
          dismissCapture()
          navigate(`/media/${result.id}`)
        }
      } catch (err) {
        setError(item.id, getUploadErrorMessage(err, t, t('upload.errors.failed')))
      }
    },
    [uploadWithCapacityRetry, setComplete, setError, navigate, t]
  )

  // Starts queued rows nobody has sent yet, at most CLIENT_UPLOAD_PARALLELISM
  // at a time; each settled upload starts the next. A row parked back as
  // "queued" while it waits for server capacity is still in `started`, so it
  // is never sent twice.
  const pump = useCallback(() => {
    for (const item of useUploadStore.getState().queue) {
      if (uploadScheduler.inFlight >= CLIENT_UPLOAD_PARALLELISM) break
      if (item.status !== 'queued' || uploadScheduler.started.has(item.id)) continue
      uploadScheduler.started.add(item.id)
      uploadScheduler.inFlight += 1
      void uploadFile(item).finally(() => {
        uploadScheduler.inFlight -= 1
        uploadScheduler.started.delete(item.id)
        pump()
      })
    }
  }, [uploadFile])

  const onDrop = useCallback(
    async (acceptedFiles: File[]) => {
      pendingDrops.current += 1
      try {
        // Query immediately before admitting a drop. The cached quota makes the
        // common path cheap, while the refetch narrows the race with another tab;
        // the server remains authoritative for every accepted file.
        const latestUsage = (await refetchUsage()).data ?? usageStats
        const storage = latestUsage?.user_storage
        const pendingBytes = useUploadStore.getState().queue
          .filter((item) => item.status === 'queued' || item.status === 'uploading' || item.status === 'processing')
          .reduce((sum, item) => sum + item.file.size, 0)
        const remainingBytes = storage?.enforced
          ? Math.max(0, storage.remaining_bytes - pendingBytes)
          : undefined

        addFiles(acceptedFiles, remainingBytes)
      } finally {
        pendingDrops.current -= 1
      }

      pump()
    },
    [addFiles, pump, refetchUsage, usageStats]
  )

  const { getRootProps, getInputProps, isDragActive, open } = useDropzone({
    onDrop,
    accept: ACCEPTED_TYPES,
    maxSize: MAX_SIZE,
    noClick: true,
    noKeyboard: false,
  })

  return (
    <div className="space-y-4">
      <StorageQuotaNotice storage={usageStats?.user_storage} />
      <div
        {...getRootProps()}
        className={cn(
          'flex flex-col items-center justify-center rounded-lg border-2 border-dashed p-8 text-center transition-colors',
          isDragActive
            ? 'border-primary bg-primary/5'
            : 'border-muted-foreground/25 hover:border-muted-foreground/50'
        )}
      >
        <input {...getInputProps()} />
        <Upload className="mb-4 h-10 w-10 text-muted-foreground" />
        <p className="mb-2 text-lg font-medium">
          {t('upload.dropzone')}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={open}
          aria-label={t('upload.browseFiles')}
        >
          {t('upload.browse')}
        </Button>
        <p className="mt-3 text-xs text-muted-foreground">
          {t('upload.formats')}
        </p>
        <p className="text-xs text-muted-foreground">{t('upload.maxSize')}</p>
      </div>

      {queue.length > 0 && (
        <ul className="space-y-2" aria-label={t('upload.queueLabel')}>
          {queue.map((item) => (
            <UploadQueueItem
              key={item.id}
              item={item}
              onRemove={() => removeItem(item.id)}
            />
          ))}
        </ul>
      )}

      {rejected.length > 0 && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
          <p className="text-sm font-medium text-destructive">{t('upload.rejectedTitle')}</p>
          <ul className="mt-2 space-y-1 text-xs text-destructive" aria-label={t('upload.rejectedLabel')}>
            {rejected.map((item) => (
              <li key={`${item.file.name}-${item.reason}`}>
                <span className="font-medium">{item.file.name}</span> — {t(`upload.rejectionReasons.${item.reason}`)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

interface UploadQueueItemProps {
  item: UploadItem
  onRemove: () => void
}

// Mirror the Link's own navigation condition: a modified or non-left click
// opens the target in a new tab and leaves this page in place, so the dialog
// (and the other rows' links) must survive.
function dismissOnPlainClick(event: MouseEvent) {
  if (event.defaultPrevented) return
  if (event.button !== 0) return
  if (event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) return
  dismissCapture()
}

function UploadQueueItem({ item, onRemove }: UploadQueueItemProps) {
  const { t } = useTranslation('media')
  return (
    <li className="flex items-center gap-3 rounded-md border bg-card p-3">
      <FileAudio className="h-5 w-5 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center justify-between gap-2">
          <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
            {item.file.name}
          </p>
          <span className="shrink-0 text-xs text-muted-foreground">
            {formatBytes(item.file.size)}
          </span>
        </div>

        {item.status === 'uploading' && (
          <div
            role="progressbar"
            aria-valuenow={item.progress}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={t('upload.uploading', { name: item.file.name })}
            className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-secondary"
          >
            <div
              className="h-full rounded-full bg-primary transition-all duration-300"
              style={{ width: `${item.progress}%` }}
            />
          </div>
        )}

        {item.status === 'processing' && (
          <p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t('upload.processing')}
          </p>
        )}

        {item.status === 'error' && item.error && (
          <p className="mt-1 flex items-center gap-1 text-xs text-destructive">
            <AlertCircle className="h-3 w-3" />
            {item.error}
          </p>
        )}

        {item.status === 'complete' && (
          <p className="mt-1 flex items-center gap-1 text-xs text-success">
            <CheckCircle className="h-3 w-3" />
            {t('upload.complete')}
            {item.mediaId && (
              <Link
                to={`/media/${item.mediaId}`}
                onClick={dismissOnPlainClick}
                className="ml-1 font-medium underline underline-offset-2"
                aria-label={t('upload.viewFileNamed', { name: item.file.name })}
              >
                {t('upload.viewFile')}
              </Link>
            )}
          </p>
        )}
      </div>

      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7 shrink-0"
        onClick={onRemove}
        aria-label={t('upload.remove', { name: item.file.name })}
      >
        <X className="h-4 w-4" />
      </Button>
    </li>
  )
}
