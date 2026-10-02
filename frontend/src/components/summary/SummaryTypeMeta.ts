import {
  CheckCheck,
  FileQuestion,
  MessageSquareQuote,
  ScrollText,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'
import type { SummaryType } from '@/types/summary'

export interface SummaryTypeMetaEntry {
  // `summary` namespace key resolving to the short, compact type label.
  shortLabelKey: string
  // `summary` namespace key resolving to the full type name, for headings and
  // for pickers where a two-word chip would not be enough to choose by.
  labelKey: string
  icon: LucideIcon
  // Order in which types are visually presented inside a briefing row.
  order: number
}

const FALLBACK_META: SummaryTypeMetaEntry = {
  shortLabelKey: 'typeMeta.fallback',
  labelKey: 'typeMeta.fallback',
  icon: FileQuestion,
  order: 99,
}

const META_BY_TYPE: Record<SummaryType, SummaryTypeMetaEntry> = {
  general: {
    shortLabelKey: 'typeMeta.general',
    labelKey: 'types.general',
    icon: ScrollText,
    order: 1,
  },
  key_points: {
    shortLabelKey: 'typeMeta.keyPoints',
    labelKey: 'types.keyPoints',
    icon: Sparkles,
    order: 2,
  },
  action_items: {
    shortLabelKey: 'typeMeta.actions',
    labelKey: 'types.actionItems',
    icon: CheckCheck,
    order: 3,
  },
  q_and_a: {
    shortLabelKey: 'typeMeta.qAndA',
    labelKey: 'types.questionsAnswers',
    icon: MessageSquareQuote,
    order: 4,
  },
}

// Safe lookup — returns a fallback for unknown types so a single unrecognized
// summary cannot crash the page if the backend adds a new type ahead of the UI.
export function getSummaryTypeMeta(type: string): SummaryTypeMetaEntry {
  return META_BY_TYPE[type as SummaryType] ?? FALLBACK_META
}

export const SUMMARY_TYPE_META = META_BY_TYPE
export const SUMMARY_TYPE_ORDER: SummaryType[] = ['general', 'key_points', 'action_items', 'q_and_a']
