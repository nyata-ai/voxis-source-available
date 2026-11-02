import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Mic, Upload as UploadIcon, type LucideIcon } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { UploadDropzone } from '@/components/media/UploadDropzone'
import { dismissCapture, useCaptureStore } from '@/stores/capture'
import { routes, type CaptureTab } from '@/config/routes'
import { INTAKE_DISC_CLASS, type IntakeChannel } from '@/lib/intake-tone'
import { cn } from '@/lib/utils'

export function CaptureModal() {
  const { t } = useTranslation('oss')
  const open = useCaptureStore((state) => state.open)
  const tab = useCaptureStore((state) => state.tab)
  const setTab = useCaptureStore((state) => state.setTab)
  const handleOpenChange = useCallback((next: boolean) => {
    if (!next) dismissCapture()
  }, [])

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('capture.title')}</DialogTitle>
          <DialogDescription>{t('capture.description')}</DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as CaptureTab)}>
          <TabsList className="grid h-auto w-full grid-cols-2 items-stretch">
            <CaptureTabTrigger value="upload" tone="upload" icon={UploadIcon} label={t('capture.upload')} />
            <CaptureTabTrigger value="record" tone="record" icon={Mic} label={t('capture.record')} />
          </TabsList>
          <TabsContent value="upload">
            <UploadDropzone />
          </TabsContent>
          <TabsContent value="record" className="space-y-4 py-2">
            <p className="text-sm text-muted-foreground">{t('capture.recordBody')}</p>
            <Button asChild className="w-full sm:w-auto">
              <Link to={routes.record.path} onClick={dismissCapture}>
                <Mic className="mr-2 h-4 w-4" aria-hidden="true" />
                {t('capture.openRecorder')}
              </Link>
            </Button>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}

interface CaptureTabTriggerProps {
  value: CaptureTab
  tone: IntakeChannel
  icon: LucideIcon
  label: string
}

/** A capture tab with its channel's intake colour as a disc behind the icon —
 * the same peach and mint the Desk tiles and Library rows use. Below `sm`
 * the disc sits over the label, which wraps: beside it, 47 px clips most labels
 * in most languages. */
function CaptureTabTrigger({ value, tone, icon: Icon, label }: CaptureTabTriggerProps) {
  return (
    <TabsTrigger
      value={value}
      className="min-w-0 flex-col justify-start gap-1 whitespace-normal px-1.5 py-1.5 sm:flex-row sm:justify-center sm:gap-2 sm:whitespace-nowrap sm:px-3"
    >
      <span
        className={cn('flex h-5 w-5 shrink-0 items-center justify-center rounded-full', INTAKE_DISC_CLASS[tone])}
        aria-hidden="true"
      >
        <Icon className="h-3 w-3" />
      </span>
      <span className="min-w-0 max-w-full text-center leading-tight [overflow-wrap:anywhere] sm:truncate">{label}</span>
    </TabsTrigger>
  )
}
