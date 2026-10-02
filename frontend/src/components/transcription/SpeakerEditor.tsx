import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Loader2, Check, ChevronDown, X, Sparkles } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Chip } from '@/components/ui/chip'
import { Panel, PanelTitle, PANEL_ROW_CLASS } from '@/components/ui/panel'
import { cn } from '@/lib/utils'
import {
  useUpdateSpeakers,
  useGenerateSpeakerSuggestions,
  useDismissSpeakerSuggestion,
} from '@/hooks/useTranscription'
import { toast } from '@/lib/toast'
import type { SpeakerSuggestion } from '@/types/transcription'

/** Rows shown before the panel folds the rest behind "Show all". Four is what
 *  the left column can carry without pushing the forensics panel off-screen;
 *  a ten-speaker hearing is otherwise a wall of inputs above everything else. */
const COLLAPSED_ROWS = 4

interface SpeakerEditorProps {
  transcriptionId: string
  speakerMap: Record<string, string>
  speakerCount: number
  suggestedSpeakerMap?: Record<string, SpeakerSuggestion>
  suggestionsGenerated?: boolean
}

// A non-empty confirmed name takes precedence over any suggestion for a row.
function confirmedName(speakerMap: Record<string, string>, key: string): string {
  const value = speakerMap[key]
  return typeof value === 'string' && value.trim() !== '' ? value : ''
}

export function SpeakerEditor({
  transcriptionId,
  speakerMap,
  speakerCount,
  suggestedSpeakerMap,
  suggestionsGenerated,
}: SpeakerEditorProps) {
  const { t } = useTranslation('transcription')
  const updateMutation = useUpdateSpeakers(transcriptionId)
  const { mutate: generateSuggestions, isPending: isGenerating } =
    useGenerateSpeakerSuggestions(transcriptionId)
  const dismissMutation = useDismissSpeakerSuggestion(transcriptionId)

  // Display value per row, seeded from confirmed names. Suggestions are NOT
  // baked into `inputs`; they're shown separately so the chip and the
  // accept/dismiss affordances can render until the user acts on them.
  const buildConfirmed = () => {
    const next: Record<string, string> = {}
    for (let i = 0; i < speakerCount; i++) {
      const key = String(i)
      next[key] = confirmedName(speakerMap, key)
    }
    return next
  }

  const [inputs, setInputs] = useState<Record<string, string>>(buildConfirmed)
  const [showLowConfidence, setShowLowConfidence] = useState(false)
  const [showAllRows, setShowAllRows] = useState(false)
  // Per-row dismiss-in-flight keys; a shared mutation flag would disable every
  // row's Dismiss button while one is dismissing.
  const [pendingDismissals, setPendingDismissals] = useState<Set<string>>(new Set())
  // Rows the user is actively editing; their typed value must survive refetches.
  const dirtyRef = useRef<Set<string>>(new Set())

  // Sync confirmed names from props on refetch, but never clobber a dirty row.
  useEffect(() => {
    setInputs((prev) => {
      const next: Record<string, string> = {}
      for (let i = 0; i < speakerCount; i++) {
        const key = String(i)
        next[key] = dirtyRef.current.has(key) ? (prev[key] ?? '') : confirmedName(speakerMap, key)
      }
      return next
    })
  }, [speakerMap, speakerCount])

  // Auto-generate suggestions once per visit when none exist yet. The ref guard
  // (keyed by transcriptionId) prevents StrictMode double-invoke and refetch
  // loops; after success `suggestions_generated` flips true so this won't re-fire.
  // `generateSuggestions` has a stable identity (TanStack v5), and the guard keeps
  // re-runs idempotent, so listing it honestly is safe.
  const autoGenRef = useRef<string | null>(null)
  useEffect(() => {
    if (autoGenRef.current === transcriptionId) return
    const hasSuggestions = !!suggestedSpeakerMap && Object.keys(suggestedSpeakerMap).length > 0
    if (speakerCount > 0 && suggestionsGenerated === false && !hasSuggestions) {
      autoGenRef.current = transcriptionId
      // Best-effort background action: never toast, never crash. TanStack clears
      // isPending on error automatically; log for diagnostics only.
      generateSuggestions(undefined, {
        onError: (err) => {
          if (import.meta.env.DEV) {
            console.debug('speaker suggestion auto-generate failed', err)
          }
        },
      })
    }
  }, [
    transcriptionId,
    speakerCount,
    suggestionsGenerated,
    suggestedSpeakerMap,
    generateSuggestions,
  ])

  const isVisibleSuggestion = (s: SpeakerSuggestion | undefined): s is SpeakerSuggestion => {
    if (!s) return false
    return s.confidence !== 'low' || showLowConfidence
  }

  const commit = (key: string, value: string) => {
    dirtyRef.current.delete(key)
    // Build a whole-map replace, but OMIT indices whose (trimmed) value is empty.
    // Persisting `''` for untouched rows would store blank labels the display
    // can't distinguish from "unnamed", so only real names are sent. Clearing a
    // previously-confirmed name to empty drops its index → the backend removes it
    // → that speaker reverts to unnamed (the intended "un-name" behavior).
    const map: Record<string, string> = {}
    for (let i = 0; i < speakerCount; i++) {
      const k = String(i)
      const next = k === key ? value : (inputs[k] ?? '')
      if (next.trim() !== '') map[k] = next
    }
    updateMutation.mutate(map, {
      onError: () => {
        // Re-mark dirty so the optimistic value in `inputs` survives a refetch,
        // letting the user retry without retyping.
        dirtyRef.current.add(key)
        toast.error(t('speakers.saveFailed'))
      },
    })
  }

  const handleChange = (key: string, value: string) => {
    dirtyRef.current.add(key)
    setInputs((prev) => ({ ...prev, [key]: value }))
  }

  const handleCommit = (key: string) => {
    if (!dirtyRef.current.has(key)) return
    commit(key, inputs[key] ?? '')
  }

  const handleAccept = (key: string, name: string) => {
    // Optimistically show the accepted name; PATCH carries the full current map.
    setInputs((prev) => ({ ...prev, [key]: name }))
    commit(key, name)
  }

  const handleDismiss = (key: string) => {
    setPendingDismissals((prev) => new Set(prev).add(key))
    dismissMutation.mutate(key, {
      onError: () => toast.error(t('speakers.dismissFailed')),
      onSettled: () => {
        setPendingDismissals((prev) => {
          const next = new Set(prev)
          next.delete(key)
          return next
        })
      },
    })
  }

  const hasLowConfidence =
    !!suggestedSpeakerMap && Object.values(suggestedSpeakerMap).some((s) => s?.confidence === 'low')
  const isCollapsible = speakerCount > COLLAPSED_ROWS
  const visibleCount = isCollapsible && !showAllRows ? COLLAPSED_ROWS : speakerCount

  return (
    <Panel>
      <div className="flex items-center gap-2 pb-3">
        <PanelTitle className="pb-0">{t('speakers.title')}</PanelTitle>
        {updateMutation.isPending && (
          <Loader2
            className="h-4 w-4 animate-spin text-muted-foreground"
            aria-label={t('speakers.saving')}
          />
        )}
      </div>

      {isGenerating && (
        <p
          className="flex items-center gap-1.5 pb-3 text-xs text-ink-muted dark:text-muted-foreground"
          role="status"
        >
          <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
          {t('speakers.findingNames')}
        </p>
      )}

      {Array.from({ length: visibleCount }, (_, i) => {
        const key = String(i)
        const confirmed = confirmedName(speakerMap, key)
        const rawSuggestion = suggestedSpeakerMap?.[key]
        const suggestion =
          !confirmed && isVisibleSuggestion(rawSuggestion) ? rawSuggestion : undefined
        return (
          <SpeakerRow
            key={key}
            index={i}
            value={inputs[key] ?? ''}
            suggestion={suggestion}
            onChange={(v) => handleChange(key, v)}
            onCommit={() => handleCommit(key)}
            onAccept={(name) => handleAccept(key, name)}
            onDismiss={() => handleDismiss(key)}
            dismissPending={pendingDismissals.has(key)}
            t={t}
          />
        )
      })}

      {isCollapsible && (
        <button
          type="button"
          onClick={() => setShowAllRows((previous) => !previous)}
          aria-expanded={showAllRows}
          className={cn(
            PANEL_ROW_CLASS,
            'flex w-full items-center justify-between gap-3 text-left text-sm text-foreground',
            'transition-colors hover:text-coral focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
          )}
        >
          {showAllRows
            ? t('speakers.showFewer')
            : t('speakers.showAll', { count: speakerCount })}
          <ChevronDown
            className={cn(
              'h-4 w-4 shrink-0 text-ink-muted transition-transform dark:text-muted-foreground',
              showAllRows && 'rotate-180'
            )}
            aria-hidden="true"
          />
        </button>
      )}

      {hasLowConfidence && (
        <div className={cn(PANEL_ROW_CLASS, 'flex items-center gap-2.5')}>
          <Checkbox
            id="show-low-confidence"
            checked={showLowConfidence}
            onCheckedChange={(checked) => setShowLowConfidence(checked === true)}
          />
          <Label
            htmlFor="show-low-confidence"
            className="text-[13px] font-normal text-ink-muted dark:text-muted-foreground"
          >
            {t('speakers.showLowConfidence')}
          </Label>
        </div>
      )}
    </Panel>
  )
}

interface SpeakerRowProps {
  index: number
  value: string
  suggestion?: SpeakerSuggestion
  onChange: (value: string) => void
  onCommit: () => void
  onAccept: (name: string) => void
  onDismiss: () => void
  dismissPending: boolean
  t: TFunction<'transcription'>
}

function SpeakerRow({
  index,
  value,
  suggestion,
  onChange,
  onCommit,
  onAccept,
  onDismiss,
  dismissPending,
  t,
}: SpeakerRowProps) {
  const key = String(index)
  const speakerNumber = index + 1
  // Suggested = no typed/confirmed value but a visible suggestion exists. The
  // suggestion renders as a PLACEHOLDER (never the value), so the first keystroke
  // replaces it cleanly instead of appending to it.
  const isSuggested = !!suggestion && value.trim() === ''

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      onCommit()
    }
  }

  return (
    <div className={cn(PANEL_ROW_CLASS, 'grid grid-cols-[96px_minmax(0,1fr)] items-center gap-3')}>
      <Label
        htmlFor={`speaker-${key}`}
        className="text-[13px] font-normal text-ink-muted dark:text-muted-foreground"
      >
        {t('speakers.speakerLabel', { index: speakerNumber })}
      </Label>
      <div className="min-w-0 space-y-2">
        <Input
          id={`speaker-${key}`}
          value={value}
          placeholder={
            isSuggested ? suggestion.name : t('speakers.speakerLabel', { index: speakerNumber })
          }
          onChange={(e) => onChange(e.target.value)}
          onBlur={onCommit}
          onKeyDown={handleKeyDown}
          className="h-9 rounded-sm"
          aria-describedby={isSuggested ? `speaker-${key}-evidence` : undefined}
        />
        {isSuggested && (
          <div className="flex flex-wrap items-center gap-2">
            {/* The shared chip vocabulary, at the size a person's name needs:
                13 px in body ink rather than the 12 px muted label a chip
                normally carries. */}
            <Chip
              className="min-h-8 text-[13px] font-normal text-foreground dark:text-foreground"
              title={suggestion.evidence}
            >
              <Sparkles
                className="h-3.5 w-3.5 shrink-0 text-ink-muted dark:text-muted-foreground"
                aria-hidden="true"
              />
              {suggestion.name}
            </Chip>
            <span id={`speaker-${key}-evidence`} className="sr-only">
              {t('speakers.evidence', {
                name: suggestion.name,
                evidence: suggestion.evidence,
                confidence: suggestion.confidence,
              })}
            </span>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="h-11 w-11 rounded-sm sm:h-8 sm:w-8"
              onClick={() => onAccept(suggestion.name)}
              aria-label={t('speakers.acceptLabel', { name: suggestion.name, index: speakerNumber })}
            >
              <Check className="h-4 w-4" aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-11 w-11 rounded-sm sm:h-8 sm:w-8"
              disabled={dismissPending}
              onClick={onDismiss}
              aria-label={t('speakers.dismissLabel', { index: speakerNumber })}
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
