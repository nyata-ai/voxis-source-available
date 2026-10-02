import { AlertCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { negotiateCodec } from '@/hooks/useMediaRecorder'

interface BrowserCompatCheckProps {
  children: React.ReactNode
}

/**
 * Checks MediaRecorder support and codec availability.
 * Renders children if supported, otherwise shows an incompatibility message.
 */
export function BrowserCompatCheck({ children }: BrowserCompatCheckProps) {
  const { t } = useTranslation('recording')
  const hasMediaRecorder = typeof MediaRecorder !== 'undefined'
  const hasGetUserMedia =
    typeof navigator !== 'undefined' &&
    typeof navigator.mediaDevices !== 'undefined' &&
    typeof navigator.mediaDevices.getUserMedia === 'function'

  const codec = hasMediaRecorder ? negotiateCodec() : { supported: false }

  const isSupported = hasMediaRecorder && hasGetUserMedia && codec.supported

  if (!isSupported) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 py-16 text-center" role="alert">
        <AlertCircle className="h-12 w-12 text-muted-foreground" />
        <div className="space-y-2">
          <h2 className="text-xl font-semibold">{t('compat.title')}</h2>
          <p className="text-muted-foreground max-w-md">
            {t('compat.message')}
          </p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}
