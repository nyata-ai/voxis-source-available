import type { StructuredSummaryContent, SummaryDetail, SummaryType } from '@/types/summary'

export interface QuestionAnswerItem {
  question: string
  answer: string
}

export function parseQuestionAnswerContent(content: string): QuestionAnswerItem[] | null {
  const lines = content
    .trim()
    .split(/\r?\n/)
    .map((line) => line.trim())

  if (lines.length === 0 || lines.every((line) => !line)) return null

  const items: QuestionAnswerItem[] = []
  let current: QuestionAnswerItem | null = null
  let currentField: 'question' | 'answer' | null = null

  for (const line of lines) {
    if (!line) continue

    const questionMatch = line.match(/^Q:\s*(.*)$/)
    if (questionMatch) {
      if (current) {
        if (!current.question.trim() || !current.answer.trim()) return null
        items.push({
          question: current.question.trim(),
          answer: current.answer.trim(),
        })
      }

      current = { question: questionMatch[1], answer: '' }
      currentField = 'question'
      continue
    }

    const answerMatch = line.match(/^A:\s*(.*)$/)
    if (answerMatch) {
      if (!current || !current.question.trim()) return null
      if (currentField === 'answer' && current.answer.trim()) {
        current.answer = `${current.answer}\n${line}`
        continue
      }
      current.answer = answerMatch[1]
      currentField = 'answer'
      continue
    }

    if (!current || !currentField) return null
    current[currentField] = `${current[currentField]}\n${line}`
  }

  if (!current || !current.question.trim() || !current.answer.trim()) return null

  items.push({
    question: current.question.trim(),
    answer: current.answer.trim(),
  })

  return items
}

type JsonRecord = Record<string, unknown>

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string'
}

function hasCitationFields(value: unknown): value is JsonRecord {
  return (
    isRecord(value) &&
    typeof value.id === 'string' &&
    Array.isArray(value.citation_ids) &&
    value.citation_ids.length > 0 &&
    value.citation_ids.every((citation) => typeof citation === 'string')
  )
}

function hasTextCitationItem(value: unknown): value is JsonRecord {
  return (
    hasCitationFields(value) && typeof value.text === 'string' && isNullableString(value.speaker)
  )
}

function hasStructuredSummaryBase(value: JsonRecord): boolean {
  return (
    value.schema_version === 'structured_summary_v1' && typeof value.matrix_language === 'string'
  )
}

function isStructuredContentForType(value: JsonRecord, summaryType: SummaryType): boolean {
  if (!hasStructuredSummaryBase(value) || !Array.isArray(value.items ?? value.paragraphs))
    return false

  const items = (value.items ?? value.paragraphs) as unknown[]
  if (summaryType === 'action_items') {
    return items.every(
      (item) =>
        hasTextCitationItem(item) &&
        isNullableString(item.owner) &&
        isNullableString(item.deadline) &&
        ['action_item', 'decision', 'next_step', 'open_issue'].includes(item.kind as string)
    )
  }
  if (summaryType === 'q_and_a') {
    return items.every(
      (item) =>
        hasCitationFields(item) &&
        typeof item.question === 'string' &&
        isNullableString(item.question_speaker) &&
        typeof item.answer === 'string' &&
        isNullableString(item.answer_speaker) &&
        isNullableString(item.answer_exact_quote) &&
        ['answered', 'partially_answered', 'conflicting', 'unanswered'].includes(
          item.answer_status as string
        )
    )
  }
  return items.every(hasTextCitationItem)
}

function parseStructuredContent(value: unknown): unknown {
  if (typeof value !== 'string') return value
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

/** Returns structured content only when it matches the selected summary type. */
export function getStructuredSummaryContent(
  summary: SummaryDetail
): StructuredSummaryContent | null {
  const value = parseStructuredContent(summary.structured_content)
  if (!isRecord(value) || !isStructuredContentForType(value, summary.summary_type)) return null
  return value as unknown as StructuredSummaryContent
}
