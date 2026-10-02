import { useState, useRef, useCallback, useLayoutEffect, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Pencil } from 'lucide-react'
import { cn } from '@/lib/utils'

interface InlineEditableTextProps {
  value: string
  onSave: (newValue: string) => void
  placeholder?: string
  ariaLabel?: string
  multiline?: boolean
  maxLength?: number
  className?: string
  inputClassName?: string
  as?: 'h1' | 'h2' | 'p' | 'span'
  stopPropagation?: boolean
}

export function InlineEditableText({
  value,
  onSave,
  placeholder = '',
  ariaLabel,
  multiline = false,
  maxLength,
  className,
  inputClassName,
  as: Tag = 'p',
  stopPropagation = false,
}: InlineEditableTextProps) {
  const { t } = useTranslation('media')
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null)
  const savedByKeyRef = useRef(false)

  useLayoutEffect(() => {
    if (editing) {
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [editing])

  const startEditing = useCallback(() => {
    setDraft(value)
    setEditing(true)
  }, [value])

  const save = useCallback(() => {
    const trimmed = draft.trim()
    setEditing(false)
    if (trimmed !== value) {
      onSave(trimmed)
    }
  }, [draft, value, onSave])

  const cancel = useCallback(() => {
    setEditing(false)
    setDraft(value)
  }, [value])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (stopPropagation) {
        e.stopPropagation()
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        savedByKeyRef.current = true
        cancel()
      } else if (e.key === 'Enter') {
        if (multiline && !e.ctrlKey && !e.metaKey) return
        e.preventDefault()
        savedByKeyRef.current = true
        save()
      }
    },
    [cancel, save, multiline, stopPropagation],
  )

  const handleBlur = useCallback(() => {
    if (savedByKeyRef.current) {
      savedByKeyRef.current = false
      return
    }
    save()
  }, [save])

  if (editing) {
    const shared = {
      ref: inputRef as React.RefObject<HTMLInputElement & HTMLTextAreaElement>,
      value: draft,
      'aria-label': ariaLabel,
      onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setDraft(e.target.value),
      onBlur: handleBlur,
      onClick: stopPropagation
        ? (e: React.MouseEvent<HTMLInputElement | HTMLTextAreaElement>) => e.stopPropagation()
        : undefined,
      onKeyDown: handleKeyDown,
      maxLength,
      className: cn(
        'w-full rounded-md border border-input bg-background px-3 py-1.5 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        inputClassName ?? className,
      ),
    }

    if (multiline) {
      return (
        <div>
          <textarea {...shared} rows={3} />
          {maxLength && (
            <p className="mt-1 text-xs text-muted-foreground text-right">
              {draft.length}/{maxLength}
            </p>
          )}
        </div>
      )
    }

    return <input type="text" {...shared} />
  }

  const displayValue = value || placeholder
  const isEmpty = !value

  return (
    <button
      type="button"
      aria-label={ariaLabel}
      onClick={(e) => {
        if (stopPropagation) {
          e.stopPropagation()
        }
        startEditing()
      }}
      onKeyDown={(e) => {
        if (stopPropagation) {
          e.stopPropagation()
        }
      }}
      className={cn(
        'group inline-flex items-center gap-2 rounded-md px-1 -mx-1 transition-all cursor-text text-left',
        'border-b border-dashed border-transparent hover:border-muted-foreground/30',
        'hover:bg-accent/50',
        className,
      )}
      title={t('inlineEdit.clickToEdit')}
    >
      <Tag className={cn('inline', isEmpty && 'text-muted-foreground')}>{displayValue}</Tag>
      <Pencil className="h-3.5 w-3.5 shrink-0 text-muted-foreground/40 group-hover:text-muted-foreground transition-colors" />
    </button>
  )
}
