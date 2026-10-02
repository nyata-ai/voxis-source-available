import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight, Shield, ShieldAlert, ShieldQuestion, Hash } from 'lucide-react'
import { Panel } from '@/components/ui/panel'
import { statusTone, TONE_DOT_CLASS } from '@/lib/status-tone'
import { cn } from '@/lib/utils'
import type { AudioAnalysis } from '@/types/media'

// The verdict rides the collapsed row as a shield glyph plus words — no colour
// badge. Colour alone was never the signal here, and a green pill above a
// transcript reads as a guarantee the checks do not make.
const trustConfig = {
  high: { labelKey: 'analysis.trust.high', icon: Shield, iconClass: 'text-success' },
  medium: { labelKey: 'analysis.trust.medium', icon: ShieldAlert, iconClass: 'text-warning' },
  low: { labelKey: 'analysis.trust.low', icon: ShieldAlert, iconClass: 'text-destructive' },
  unknown: {
    labelKey: 'analysis.trust.unknown',
    icon: ShieldQuestion,
    iconClass: 'text-ink-muted dark:text-muted-foreground',
  },
}

interface AudioAnalysisCardProps {
  analysis: AudioAnalysis
  fileHash?: string
}

/**
 * Forensics, folded. Integrity is a reassurance the reader consults, not
 * something they read on the way to the transcript, so the panel opens as one
 * 44 px row — shield, verdict, chevron — and keeps every finding exactly as it
 * was underneath.
 */
export function AudioAnalysisCard({ analysis, fileHash }: AudioAnalysisCardProps) {
  const { t } = useTranslation('media')
  const [isOpen, setIsOpen] = useState(false)

  const trust = trustConfig[analysis.trust_level] ?? trustConfig.unknown
  const TrustIcon = trust.icon

  return (
    <Panel padded={false}>
      <button
        type="button"
        onClick={() => setIsOpen((previous) => !previous)}
        aria-expanded={isOpen}
        // 18 px vertical, 20 px horizontal per the artboard's collapsed row.
        className="flex min-h-11 w-full items-center justify-between gap-3 px-5 py-[18px] text-left transition-colors hover:text-coral focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="inline-flex items-center gap-2.5 text-sm">
          <TrustIcon className={cn('h-4 w-4 shrink-0', trust.iconClass)} aria-hidden="true" />
          {/* Three nodes, not one string: the title and the verdict stay
              independently addressable, and the separator is decoration a
              screen reader has no use for. */}
          <span>{t('analysis.title')}</span>
          <span aria-hidden="true" className="text-ink-muted dark:text-muted-foreground">
            ·
          </span>
          <span>{t(trust.labelKey)}</span>
        </span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-ink-muted transition-transform dark:text-muted-foreground',
            isOpen && 'rotate-180'
          )}
          aria-hidden="true"
        />
      </button>

      {isOpen && <AnalysisDetails analysis={analysis} fileHash={fileHash} />}
    </Panel>
  )
}

/** Everything behind the collapsed row, in its own component so the panel above
 *  stays one screen: a shield, a verdict and a chevron. */
function AnalysisDetails({ analysis, fileHash }: AudioAnalysisCardProps) {
  const { t } = useTranslation('media')
  const hasTechnicalDetails =
    analysis.format_name || analysis.codec_name || analysis.encoder || analysis.recording_source
  const passCount = analysis.findings.filter((f) => f.status === 'pass').length
  const totalChecks = analysis.findings.length

  return (
    <div className="space-y-3 border-t px-5 py-4">
      {totalChecks > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('analysis.checksPassed', { passed: passCount, total: totalChecks })}
        </p>
      )}

      {/* Key metadata — human-friendly */}
      {analysis.recording_date && (
        <div className="flex justify-between gap-4 text-sm">
          <span className="text-muted-foreground">{t('analysis.recorded')}</span>
          <span className="font-medium">{analysis.recording_date}</span>
        </div>
      )}

      {totalChecks > 0 && (
        <Disclosure label={t('analysis.integrityDetails')}>
          <ul className="mt-2 space-y-1.5 pl-1">
            {analysis.findings.map((f, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                <FindingDot status={f.status} />
                <span className="text-muted-foreground">{f.summary}</span>
              </li>
            ))}
          </ul>
        </Disclosure>
      )}

      {hasTechnicalDetails && (
        <Disclosure label={t('analysis.technicalDetails')}>
          <div className="mt-2 space-y-1.5 pl-1">
            {analysis.format_name && (
              <TechRow label={t('analysis.tech.format')} value={analysis.format_name} />
            )}
            {analysis.codec_name && (
              <TechRow label={t('analysis.tech.codec')} value={analysis.codec_name} />
            )}
            {analysis.encoder && (
              <TechRow label={t('analysis.tech.encoder')} value={analysis.encoder} />
            )}
            {analysis.recording_source && (
              <TechRow label={t('analysis.tech.source')} value={analysis.recording_source} />
            )}
          </div>
        </Disclosure>
      )}

      {fileHash && (
        <Disclosure
          label={t('analysis.fileHash')}
          icon={<Hash className="h-3 w-3" aria-hidden="true" />}
        >
          <p className="ml-1 mt-1 break-all font-mono text-xs text-muted-foreground">
            {fileHash}
          </p>
        </Disclosure>
      )}

      <p className="border-t pt-1 text-xs text-muted-foreground">{analysis.disclaimer}</p>
    </div>
  )
}

/** One detail the reader can open inside the expanded panel. Three of these —
 *  findings, technical details, file hash — were the same block three times. */
function Disclosure({
  label,
  icon,
  children,
}: {
  label: string
  icon?: ReactNode
  children: ReactNode
}) {
  const [isOpen, setIsOpen] = useState(false)
  const Chevron = isOpen ? ChevronDown : ChevronRight
  return (
    <div>
      <button
        type="button"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((previous) => !previous)}
        className="inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <Chevron className="h-3.5 w-3.5" aria-hidden="true" />
        {icon}
        {label}
      </button>
      {isOpen && children}
    </div>
  )
}

function TechRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 justify-between gap-4 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{value}</span>
    </div>
  )
}

/** The finding's tone as one solid mark. The status→colour decision lives in
 *  `lib/status-tone`, never here — one status language for the whole app. */
function FindingDot({ status }: { status: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn('mt-1.5 h-2 w-2 shrink-0 rounded-full', TONE_DOT_CLASS[statusTone(status)])}
    />
  )
}
