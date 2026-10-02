import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Mic, Upload as UploadIcon } from 'lucide-react'
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
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('capture.title')}</DialogTitle>
          <DialogDescription>{t('capture.description')}</DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={(value) => setTab(value as CaptureTab)}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="upload">
              <UploadIcon className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
              {t('capture.upload')}
            </TabsTrigger>
            <TabsTrigger value="record">
              <Mic className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
              {t('capture.record')}
            </TabsTrigger>
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
