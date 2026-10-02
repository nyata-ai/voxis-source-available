import { Construction } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

interface PlaceholderPageProps {
  title: string
  description?: string
}

export function PlaceholderPage({ title, description }: PlaceholderPageProps) {
  const { t } = useTranslation('common')

  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <Card className="max-w-md w-full text-center">
        <CardHeader>
          <Construction className="h-12 w-12 text-muted-foreground mx-auto mb-2" aria-hidden="true" />
          <CardTitle>{title}</CardTitle>
          <CardDescription>
            {description || t('placeholder.defaultDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {t('placeholder.body')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
