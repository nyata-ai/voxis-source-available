import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface PaginationControlsProps {
  offset: number
  limit: number
  total: number
  onPageChange: (newOffset: number) => void
}

export function PaginationControls({
  offset,
  limit,
  total,
  onPageChange,
}: PaginationControlsProps) {
  const { t } = useTranslation('media')
  const currentPage = Math.floor(offset / limit) + 1
  const totalPages = Math.max(1, Math.ceil(total / limit))
  const hasPrev = offset > 0
  const hasNext = offset + limit < total

  if (total <= limit) return null

  return (
    <div className="flex items-center justify-between">
      <p className="text-sm text-muted-foreground">
        {t('pagination.showing', { from: offset + 1, to: Math.min(offset + limit, total), total })}
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(Math.max(0, offset - limit))}
          disabled={!hasPrev}
          aria-label={t('pagination.previous')}
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <span className="text-sm text-muted-foreground">
          {currentPage} / {totalPages}
        </span>
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(offset + limit)}
          disabled={!hasNext}
          aria-label={t('pagination.next')}
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
