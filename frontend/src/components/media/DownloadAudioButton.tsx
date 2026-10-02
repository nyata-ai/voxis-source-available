import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { apiClient } from '@/lib/api-client'
import { toast } from '@/lib/toast'

interface DownloadAudioButtonProps {
  mediaId: string
  mediaFilename: string
  status?: string
  audioAvailable?: boolean
  /** Renders the glyph alone, for a row that already carries the label (the
   *  reader's export panel). The accessible name is unchanged. */
  iconOnly?: boolean
}

export function DownloadAudioButton({
  mediaId,
  mediaFilename,
  status = 'completed',
  audioAvailable = true,
  iconOnly = false,
}: DownloadAudioButtonProps) {
  const { t } = useTranslation('media')
  const [isDownloading, setIsDownloading] = useState(false)

  if (status !== 'completed' || !audioAvailable) return null

  const fallbackFilename = mediaFilename?.trim() || `audio-${mediaId}`

  const handleDownload = async () => {
    setIsDownloading(true)
    try {
      await apiClient.download(`/media/${mediaId}/download`, fallbackFilename)
      toast.success(t('download.success'))
    } catch {
      toast.error(t('download.error'))
    } finally {
      setIsDownloading(false)
    }
  }

  if (iconOnly) {
    return (
      <Button
        variant="ghost"
        size="icon"
        className="h-11 w-11 sm:h-9 sm:w-9"
        onClick={handleDownload}
        disabled={isDownloading}
        aria-label={t('download.label')}
      >
        {isDownloading ? (
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
        ) : (
          <Download className="h-4 w-4" aria-hidden="true" />
        )}
      </Button>
    )
  }

  return (
    <Button variant="outline" size="sm" onClick={handleDownload} disabled={isDownloading}>
      {isDownloading ? t('download.downloading') : t('download.label')}
    </Button>
  )
}
