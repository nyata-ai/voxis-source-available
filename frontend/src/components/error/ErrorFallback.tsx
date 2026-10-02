import { FallbackProps } from 'react-error-boundary'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function ErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  const { t } = useTranslation('common')
  return (
    <div className="min-h-screen flex items-center justify-center p-4" role="alert">
      <Card className="max-w-md w-full">
        <CardHeader className="text-center">
          <AlertTriangle
            className="h-12 w-12 text-destructive mx-auto mb-2"
            aria-hidden="true"
          />
          <CardTitle>{t('errorBoundary.title')}</CardTitle>
          <CardDescription>{t('errorBoundary.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {import.meta.env.DEV && (
            <details className="text-sm text-muted-foreground">
              <summary className="cursor-pointer">{t('errorBoundary.details')}</summary>
              <pre className="mt-2 p-2 bg-muted rounded-md text-xs overflow-auto">
                {error instanceof Error ? error.message : String(error)}
              </pre>
            </details>
          )}
          <Button onClick={resetErrorBoundary} className="w-full">
            <RefreshCw className="h-4 w-4 mr-2" aria-hidden="true" />
            {t('actions.retry')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
