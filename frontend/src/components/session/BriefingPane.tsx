import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, RefreshCw, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SummaryViewer } from '@/components/summary/SummaryViewer'
import { getSummaryTypeMeta } from '@/components/summary/SummaryTypeMeta'
import { useSummaryDetail } from '@/hooks/useSummary'
import { useFormatters } from '@/i18n/useFormatters'
import { cn } from '@/lib/utils'
import type { SummaryDetail, SummaryType } from '@/types/summary'
import { DEFAULT_SUMMARY_PROFILE, SUMMARY_PROFILES, type SummaryProfile } from '@/types/settings'
import { summaryErrorMessage } from './summary-errors'
import type { SessionBriefingControls, SessionLens } from './session-view'

// Resolves a `summary` namespace key to its localized string.
type TranslateFn = (key: string, options?: Record<string, unknown>) => string

export interface BriefingPaneProps {
  /** Always the full lens set — `toLenses` guarantees four, in order. */
  lenses: SessionLens[]
  briefing: SessionBriefingControls
  /** Controlled tab. The reader shell resolves the opening lens (`?lens=`, then
   *  the first ready one, then the preference) so one owner answers both the
   *  deep link and the export panel's "Briefing · {lens}" row. */
  activeLens: SummaryType
  /** Told both when the reader picks a tab and when `activeLens` named no lens
   *  in this set and the pane had to fall back — see `lensFor`. */
  onActiveLensChange: (type: SummaryType) => void
  /** Saved Settings profile used until the user chooses a local override. */
  defaultSummaryProfile?: SummaryProfile
  /** Mirrors the server rollout flag; disabled deployments keep legacy behavior. */
  summaryProfilesEnabled?: boolean
  /** Available for readers with playable audio so citation chips can seek. */
  onCitationSeek?: (startSeconds: number) => void
}

/** DOM id of one lens's body, so old `#briefing-section-<type>` anchors still
 *  land on the briefing they named (when it is the one on screen). */
function sectionId(type: SummaryType): string {
  return `briefing-section-${type}`
}

// The status→copy mapping lives in `summaryErrorMessage`, shared with both
// reader adapters so a 503 or a 429 cannot read one way in a toast and another
// way in the pane. All this adds is "no error, no line".
function briefingErrorMessage(error: unknown, t: TranslateFn, fallbackKey: string): string | null {
  if (!error) return null
  return summaryErrorMessage(error, t, fallbackKey)
}

/** The hoisted regenerate failure names its type: the generic fallback becomes
 *  "Couldn't regenerate the {type} briefing", while a specific message (a 503,
 *  a 429) keeps its own words behind the type label. */
function regenerateErrorText(error: unknown, t: TranslateFn, typeLabel: string): string | null {
  const message = briefingErrorMessage(error, t, 'briefing.regenerateError')
  if (!message) return null
  if (message === t('briefing.regenerateError')) {
    return t('briefing.regenerateTypeError', { type: typeLabel })
  }
  return `${typeLabel} · ${message}`
}

/** What one tab's dot says. Four states, and they are not the four summary
 *  statuses: "never asked for" is the one a status column cannot carry, and it
 *  is the state that decides whether the panel offers Generate. */
type LensState = 'not_generated' | 'processing' | 'ready' | 'failed'

function lensState(lens: SessionLens, isGenerating: boolean): LensState {
  if (lens.status === 'completed') return 'ready'
  if (lens.status === 'failed') return 'failed'
  // A request in flight counts as work even before its row exists: the POST
  // returns before the list refetch lands, and a tab that still read "Not
  // generated" would invite a second click on the same billable job.
  if (isGenerating || lens.summaryId) return 'processing'
  return 'not_generated'
}

const LENS_STATUS_KEY: Record<LensState, string> = {
  not_generated: 'briefing.status.notGenerated',
  processing: 'briefing.status.processing',
  ready: 'briefing.status.ready',
  failed: 'briefing.status.failed',
}

/** A 6 px mark after the tab label. Ready says nothing — a dot on every tab is
 *  a dot that means nothing — and every other state carries its words for a
 *  screen reader, which cannot see a ring or a colour. */
function LensStatusDot({ state, t }: { state: LensState; t: TranslateFn }) {
  if (state === 'ready') return null
  return (
    <>
      {state === 'processing' ? (
        <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" aria-hidden="true" />
      ) : (
        <span
          aria-hidden="true"
          className={cn(
            'h-1.5 w-1.5 shrink-0 rounded-full border',
            state === 'failed'
              ? 'border-coral bg-coral group-data-[state=active]:border-current group-data-[state=active]:bg-current'
              : 'border-current'
          )}
        />
      )}
      <span className="sr-only">{t(LENS_STATUS_KEY[state])}</span>
    </>
  )
}

/** One shared profile picker for the whole band: the choice applies to the next
 *  Generate, Generate all and Regenerate alike, so it belongs above all four
 *  tabs rather than repeated inside each one. */
function ProfileSelect({
  value,
  onChange,
  disabled,
  t,
}: {
  value: SummaryProfile
  onChange: (profile: SummaryProfile) => void
  disabled: boolean
  t: TranslateFn
}) {
  return (
    <Select
      value={value}
      onValueChange={(profile) => onChange(profile as SummaryProfile)}
      disabled={disabled}
    >
      <SelectTrigger
        className="h-11 w-full sm:h-9 sm:w-[12rem]"
        aria-label={t('profile.label')}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {SUMMARY_PROFILES.map((profile) => (
          <SelectItem key={profile} value={profile}>
            {t(`profile.${profile}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Regenerate is the recovery path after a failure or a speaker rename — a
 *  summary bakes in the labels present when its job ran. */
function RegenerateButton({
  summaryId,
  status,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
  t,
}: {
  summaryId: string
  status?: string
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  summaryProfilesEnabled: boolean
  t: TranslateFn
}) {
  if (status !== 'completed' && status !== 'failed') return null
  const isRegenerating = briefing.regeneratingId === summaryId
  // Regeneration is a single slot: while any lens is in flight every button
  // is inactive, so a second click cannot re-target `regeneratingId` mid-flight.
  // `regeneratingId` is purely in-flight state — it clears on settle either way,
  // so buttons re-enable once the request finishes. The lens that failed is
  // identified separately by `regenerateErrorId`.
  const anyInFlight = Boolean(briefing.regeneratingId)
  return (
    <Button
      variant="ghost"
      size="sm"
      className="min-h-11 sm:min-h-9"
      onClick={() =>
        summaryProfilesEnabled
          ? briefing.regenerate(summaryId, summaryProfile)
          : briefing.regenerate(summaryId)
      }
      disabled={anyInFlight}
      aria-busy={isRegenerating}
    >
      <RefreshCw
        className={`mr-1 h-4 w-4 ${isRegenerating ? 'animate-spin' : ''}`}
        aria-hidden="true"
      />
      {isRegenerating ? t('briefing.regenerating') : t('briefing.regenerate')}
    </Button>
  )
}

/** The one control a never-generated lens has, inside its own empty card: a
 *  reader looking at an empty panel is exactly the one who needs to see that
 *  the briefing can be asked for. */
function GenerateButton({
  summaryType,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
  t,
}: {
  summaryType: SummaryType
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  summaryProfilesEnabled: boolean
  t: TranslateFn
}) {
  const isGenerating = briefing.generatingType === summaryType
  const busy = Boolean(briefing.generatingType) || briefing.isGeneratingAll
  return (
    <Button
      variant="outline"
      size="sm"
      className="min-h-11 shrink-0 sm:min-h-9"
      onClick={() =>
        summaryProfilesEnabled
          ? briefing.generateType(summaryType, summaryProfile)
          : briefing.generateType(summaryType)
      }
      disabled={busy}
      aria-busy={isGenerating}
    >
      {isGenerating ? (
        <Loader2 className="mr-1 h-4 w-4 animate-spin" aria-hidden="true" />
      ) : (
        <Sparkles className="mr-1 h-4 w-4" aria-hidden="true" />
      )}
      {isGenerating
        ? t('briefing.generatingOne')
        : t('briefing.generateOne', { type: t(getSummaryTypeMeta(summaryType).labelKey) })}
    </Button>
  )
}

/** The provenance line under a finished briefing: when it was made, under which
 *  profile, and the one caveat a reader cannot check by looking — the speaker
 *  names in the prose are the model's reading of the transcript. */
function BriefingNote({
  summary,
  summaryProfilesEnabled,
}: {
  summary: SummaryDetail
  summaryProfilesEnabled: boolean
}) {
  const { t } = useTranslation('summary')
  const formatters = useFormatters()
  const date = formatters.date(summary.completed_at ?? summary.created_at)
  if (!date) return null

  const profile = summaryProfilesEnabled ? summary.summary_profile : undefined
  return (
    <p className="text-xs leading-relaxed text-ink-muted dark:text-muted-foreground">
      {profile
        ? t('briefing.note', { date, profile: t(`profile.${profile}`) })
        : t('briefing.noteWithoutProfile', { date })}
    </p>
  )
}

/** The rendered body of one lens, once its content is known (or known to be
 *  still loading). Shared by the embedded and fetched paths. */
function LensBody({
  lens,
  summary,
  isLoading,
  loadFailed,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
  onCitationSeek,
}: {
  lens: SessionLens
  summary?: SummaryDetail
  isLoading?: boolean
  loadFailed?: boolean
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  summaryProfilesEnabled: boolean
  onCitationSeek?: (startSeconds: number) => void
}) {
  const { t } = useTranslation('summary')
  const summaryId = lens.summaryId
  const status = summary?.status ?? lens.status

  // A failed fetch must not fall through to SummaryViewer's empty branch: an
  // unreachable briefing and an empty one look identical, and only one of them
  // is worth retrying.
  if (loadFailed && !summary) {
    return (
      <div className="space-y-3 rounded-lg border border-dashed p-8 text-center">
        <p role="alert" className="text-sm text-destructive">
          {t('briefing.loadError')}
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <SummaryViewer
        headless
        summary={summary}
        isLoading={isLoading}
        onCitationSeek={onCitationSeek}
        actions={
          summaryId ? (
            <RegenerateButton
              summaryId={summaryId}
              status={status}
              briefing={briefing}
              summaryProfile={summaryProfile}
              summaryProfilesEnabled={summaryProfilesEnabled}
              t={t}
            />
          ) : null
        }
      />
      {summary && status === 'completed' && (
        <BriefingNote summary={summary} summaryProfilesEnabled={summaryProfilesEnabled} />
      )}
    </div>
  )
}

/** Content-less lens: fetch it by id. Kept a separate component so the embedded
 *  and never-generated paths issue no query at all. */
function FetchedLens({
  lens,
  summaryId,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
  onCitationSeek,
}: {
  lens: SessionLens
  summaryId: string
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  summaryProfilesEnabled: boolean
  onCitationSeek?: (startSeconds: number) => void
}) {
  const { data, isLoading, isError } = useSummaryDetail(summaryId)
  return (
    <LensBody
      lens={lens}
      summary={data}
      isLoading={isLoading}
      loadFailed={isError}
      briefing={briefing}
      summaryProfile={summaryProfile}
      summaryProfilesEnabled={summaryProfilesEnabled}
      onCitationSeek={onCitationSeek}
    />
  )
}

interface LensPanelProps {
  lens: SessionLens
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  summaryProfilesEnabled: boolean
  onCitationSeek?: (startSeconds: number) => void
}

/** A lens with no summary row yet: say why there is nothing to read, and offer
 *  the one control that changes that. */
function EmptyLensBody({
  lens,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
}: LensPanelProps) {
  const { t } = useTranslation('summary')
  return (
    <div className="flex flex-col items-center gap-4 rounded-lg border border-dashed p-8 text-center">
      <p className="text-sm text-ink-muted dark:text-muted-foreground">
        {t('briefing.notGenerated')}
      </p>
      <GenerateButton
        summaryType={lens.type}
        briefing={briefing}
        summaryProfile={summaryProfile}
        summaryProfilesEnabled={summaryProfilesEnabled}
        t={t}
      />
    </div>
  )
}

function LensPanel({
  lens,
  briefing,
  summaryProfile,
  summaryProfilesEnabled,
  onCitationSeek,
}: LensPanelProps) {
  if (!lens.summaryId) {
    return (
      <EmptyLensBody
        lens={lens}
        briefing={briefing}
        summaryProfile={summaryProfile}
        summaryProfilesEnabled={summaryProfilesEnabled}
      />
    )
  }
  if (lens.summary) {
    return (
      <LensBody
        lens={lens}
        summary={lens.summary}
        briefing={briefing}
        summaryProfile={summaryProfile}
        summaryProfilesEnabled={summaryProfilesEnabled}
        onCitationSeek={onCitationSeek}
      />
    )
  }
  return (
    <FetchedLens
      lens={lens}
      summaryId={lens.summaryId}
      briefing={briefing}
      summaryProfile={summaryProfile}
      summaryProfilesEnabled={summaryProfilesEnabled}
      onCitationSeek={onCitationSeek}
    />
  )
}

/** The lens the band shows. `activeLens` decides; the first lens is the fallback
 *  for a value that names nothing in this set, because a tab band with no
 *  selected tab shows nothing at all. A fallback is reported back to the owner
 *  (see the effect in `BriefingPane`) so the shell — and the export panel's
 *  "Briefing · {lens}" row — never names a type the band is not showing. */
function lensFor(lenses: SessionLens[], activeLens: SummaryType): SessionLens | undefined {
  return lenses.find((lens) => lens.type === activeLens) ?? lenses[0]
}

/** The band's head row: the title, the shared profile picker and Generate all.
 *  Its own component only so the pane below stays readable — it owns no state,
 *  and the picker's value still lives with the pane that spends it. */
function BandHeader({
  briefing,
  summaryProfile,
  onProfileChange,
  summaryProfilesEnabled,
  busy,
  hasGeneratable,
}: {
  briefing: SessionBriefingControls
  summaryProfile: SummaryProfile
  onProfileChange: (profile: SummaryProfile) => void
  summaryProfilesEnabled: boolean
  busy: boolean
  hasGeneratable: boolean
}) {
  const { t } = useTranslation('summary')
  return (
    <div className="flex flex-wrap items-center justify-between gap-4">
      {/* Artboard: 24 px / 400 / 1.2 — `leading-tight` is 1.25. */}
      <h2 id="briefing-pane-title" className="text-2xl font-normal leading-[1.2]">
        {t('briefing.title')}
      </h2>
      <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
        {summaryProfilesEnabled && (
          <ProfileSelect
            value={summaryProfile}
            onChange={onProfileChange}
            disabled={busy}
            t={t}
          />
        )}
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 w-full sm:min-h-9 sm:w-auto"
          onClick={() =>
            summaryProfilesEnabled ? briefing.generateAll(summaryProfile) : briefing.generateAll()
          }
          disabled={busy || !hasGeneratable}
          aria-busy={briefing.isGeneratingAll}
        >
          {briefing.isGeneratingAll ? (
            <Loader2 className="mr-1 h-4 w-4 animate-spin" aria-hidden="true" />
          ) : (
            <Sparkles className="mr-1 h-4 w-4" aria-hidden="true" />
          )}
          {briefing.isGeneratingAll ? t('briefing.generatingAll') : t('briefing.generateAll')}
        </Button>
      </div>
    </div>
  )
}

/**
 * The Session Reader's briefing band: the four briefing types as one tab row
 * over one session's summaries, with the selected type's briefing beneath.
 * Each type is generated on request — nothing here fires on its own unless the
 * reader turned `auto_briefings` on.
 */
export function BriefingPane({
  lenses,
  briefing,
  activeLens,
  onActiveLensChange,
  defaultSummaryProfile = DEFAULT_SUMMARY_PROFILE,
  summaryProfilesEnabled = false,
  onCitationSeek,
}: BriefingPaneProps) {
  const { t } = useTranslation('summary')
  const [selectedSummaryProfile, setSelectedSummaryProfile] = useState<SummaryProfile>()

  const summaryProfile = selectedSummaryProfile ?? defaultSummaryProfile
  const active = lensFor(lenses, activeLens)

  // Showing a fallback silently would leave the shell naming one type while the
  // band shows another. Tell the owner instead, so one value drives the tab, the
  // export row and any `?lens=` the shell writes back.
  const fallbackType = active && active.type !== activeLens ? active.type : undefined
  useEffect(() => {
    if (fallbackType) onActiveLensChange(fallbackType)
  }, [fallbackType, onActiveLensChange])

  const hasAnyBriefing = lenses.some((lens) => lens.summaryId)
  // Only a type with no row at all. The server's generate-all skips every
  // existing row whose status is not `pending` — a failed one included — so
  // offering it for a failed lens would be a button that does nothing.
  // Recovery for those is the panel's own Regenerate.
  const hasGeneratable = lenses.some((lens) => !lens.summaryId)
  const busy =
    briefing.isGeneratingAll || Boolean(briefing.generatingType) || Boolean(briefing.regeneratingId)
  // Generate-all covers every lens, so its failure belongs above all four
  // rather than inside one of them.
  const generateError = briefingErrorMessage(briefing.error, t, 'briefing.error')
  // A per-type failure names its type through `generateTypeErrorType`, but a
  // tab band can only show one panel: parked inside the panel it would vanish
  // the moment the reader looked at another tab. It sits under the tabs so the
  // request that failed is answered wherever the reader is standing.
  const generateTypeError = briefing.generateTypeErrorType
    ? briefingErrorMessage(briefing.generateTypeError, t, 'briefing.generateTypeError')
    : null
  // A failed regenerate names its lens by summary id, and is hoisted for the
  // same reason: only the selected lens's body is mounted, so a notice parked
  // inside it is destroyed the moment the reader looks at another tab — taking
  // the only record that the request failed with it. Hoisted, it also has to
  // name its type, because "this briefing" means nothing once the reader is
  // standing somewhere else. Independent of `regeneratingId`, which clears the
  // moment the mutation settles so the buttons re-enable.
  const regenerateErrorLens = briefing.regenerateErrorId
    ? lenses.find((lens) => lens.summaryId === briefing.regenerateErrorId)
    : undefined
  const regenerateError = regenerateErrorLens
    ? regenerateErrorText(
        briefing.regenerateError,
        t,
        t(getSummaryTypeMeta(regenerateErrorLens.type).labelKey),
      )
    : null

  return (
    <section
      id="briefing"
      data-testid="briefing-band"
      aria-labelledby="briefing-pane-title"
      className="scroll-mt-20 space-y-6 pt-6"
    >
      <BandHeader
        briefing={briefing}
        summaryProfile={summaryProfile}
        onProfileChange={setSelectedSummaryProfile}
        summaryProfilesEnabled={summaryProfilesEnabled}
        busy={busy}
        hasGeneratable={hasGeneratable}
      />

      {/* Why the band is empty, in the terms that apply: work the user did not
          ask for should say so, and "no briefings yet" is misleading the moment
          four are on their way. Both lines answer a band with nothing in it, so
          both step aside once a briefing lands. */}
      {!hasAnyBriefing && (
        <p className="text-sm text-ink-muted dark:text-muted-foreground">
          {briefing.autoStarted ? t('briefing.autoStarted') : t('briefing.empty')}
        </p>
      )}

      {generateError && (
        <p role="alert" className="text-sm text-destructive">
          {generateError}
        </p>
      )}

      {active && (
        <Tabs
          value={active.type}
          onValueChange={(value) => onActiveLensChange(value as SummaryType)}
          className="space-y-6"
        >
          <TabsList
            aria-label={t('briefing.tabsLabel')}
            className="flex h-auto w-full flex-nowrap justify-start gap-2 overflow-x-auto bg-transparent p-0 text-inherit sm:flex-wrap sm:overflow-x-visible"
          >
            {lenses.map((lens) => (
              <LensTab
                key={lens.type}
                lens={lens}
                isGenerating={briefing.generatingType === lens.type}
                t={t}
              />
            ))}
          </TabsList>

          {generateTypeError && (
            <p role="alert" className="text-sm text-destructive">
              {generateTypeError}
            </p>
          )}

          {regenerateError && regenerateErrorLens && (
            <p role="alert" className="text-sm text-destructive">
              {regenerateError}
            </p>
          )}

          {lenses.map((lens) => (
            <TabsContent
              key={lens.type}
              value={lens.type}
              className="mt-0 max-w-[78ch] text-ink-body dark:text-foreground"
            >
              <div id={sectionId(lens.type)}>
                <LensPanel
                  lens={lens}
                  briefing={briefing}
                  summaryProfile={summaryProfile}
                  summaryProfilesEnabled={summaryProfilesEnabled}
                  onCitationSeek={onCitationSeek}
                />
              </div>
            </TabsContent>
          ))}
        </Tabs>
      )}
    </section>
  )
}

function LensTab({
  lens,
  isGenerating,
  t,
}: {
  lens: SessionLens
  isGenerating: boolean
  t: TranslateFn
}) {
  const meta = getSummaryTypeMeta(lens.type)
  return (
    <TabsTrigger
      value={lens.type}
      className={cn(
        'group min-h-11 shrink-0 gap-2 rounded-[6px] border-secondary bg-paper-soft px-4 py-2.5 text-[13px] font-normal text-ink-muted',
        'hover:text-foreground',
        'data-[state=active]:border-forest data-[state=active]:bg-forest data-[state=active]:font-semibold data-[state=active]:text-white',
        'dark:border-border dark:bg-muted dark:text-muted-foreground',
        'dark:data-[state=active]:border-white dark:data-[state=active]:bg-white dark:data-[state=active]:text-forest'
      )}
    >
      {t(meta.labelKey)}
      <LensStatusDot state={lensState(lens, isGenerating)} t={t} />
    </TabsTrigger>
  )
}
