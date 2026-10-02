export interface LanguageOption {
  value: string
  label: string
  labelKey: string
}

export interface LanguageGroup {
  key: string
  nameKey: string
  fallbackName: string
  languages: LanguageOption[]
}

export const LANGUAGE_GROUPS: LanguageGroup[] = [
  {
    key: 'english',
    nameKey: 'languageGroups.english',
    fallbackName: 'English',
    languages: [{ value: 'en', label: 'English', labelKey: 'languageNames.en' }],
  },
  {
    key: 'asia',
    nameKey: 'languageGroups.asia',
    fallbackName: 'Asia',
    languages: [
      { value: 'id', label: 'Indonesian', labelKey: 'languageNames.id' },
      { value: 'jv', label: 'Javanese', labelKey: 'languageNames.jv' },
      { value: 'su', label: 'Sundanese', labelKey: 'languageNames.su' },
      { value: 'ms', label: 'Malay', labelKey: 'languageNames.ms' },
      { value: 'tl', label: 'Tagalog', labelKey: 'languageNames.tl' },
      { value: 'th', label: 'Thai', labelKey: 'languageNames.th' },
      { value: 'vi', label: 'Vietnamese', labelKey: 'languageNames.vi' },
      { value: 'zh', label: 'Chinese', labelKey: 'languageNames.zh' },
      { value: 'ja', label: 'Japanese', labelKey: 'languageNames.ja' },
      { value: 'ko', label: 'Korean', labelKey: 'languageNames.ko' },
    ],
  },
  {
    key: 'europe',
    nameKey: 'languageGroups.europe',
    fallbackName: 'Europe',
    languages: [
      { value: 'fr', label: 'French', labelKey: 'languageNames.fr' },
      { value: 'de', label: 'German', labelKey: 'languageNames.de' },
      { value: 'it', label: 'Italian', labelKey: 'languageNames.it' },
      { value: 'pl', label: 'Polish', labelKey: 'languageNames.pl' },
      { value: 'pt', label: 'Portuguese', labelKey: 'languageNames.pt' },
      { value: 'ru', label: 'Russian', labelKey: 'languageNames.ru' },
      { value: 'es', label: 'Spanish', labelKey: 'languageNames.es' },
      { value: 'sv', label: 'Swedish', labelKey: 'languageNames.sv' },
      { value: 'da', label: 'Danish', labelKey: 'languageNames.da' },
      { value: 'no', label: 'Norwegian', labelKey: 'languageNames.no' },
      { value: 'fi', label: 'Finnish', labelKey: 'languageNames.fi' },
      { value: 'la', label: 'Latin', labelKey: 'languageNames.la' },
    ],
  },
  {
    key: 'middleEastAfrica',
    nameKey: 'languageGroups.middleEastAfrica',
    fallbackName: 'Middle East & Africa',
    languages: [
      { value: 'ar', label: 'Arabic', labelKey: 'languageNames.ar' },
      { value: 'he', label: 'Hebrew', labelKey: 'languageNames.he' },
      { value: 'af', label: 'Afrikaans', labelKey: 'languageNames.af' },
    ],
  },
]

export const LANGUAGES: LanguageOption[] = LANGUAGE_GROUPS.flatMap(g => g.languages)

export const MAX_LANGUAGES = 5

type TranslateFn = (key: string, options?: Record<string, unknown>) => string

export function getLanguageLabel(code: string, t?: TranslateFn): string {
  const language = LANGUAGES.find(l => l.value === code)
  if (!language) return code.toUpperCase()
  return t ? t(`common:${language.labelKey}`) : language.label
}

export function formatLanguages(languages: string[], t?: TranslateFn): string {
  if (languages.length === 1 && languages[0] === 'auto') {
    return t ? t('common:languageNames.autoDetect') : 'Auto-detect'
  }
  return languages.map((language) => getLanguageLabel(language, t)).join(', ')
}

export function formatLanguageCodes(languages: string[], t?: TranslateFn): string {
  if (languages.length === 1 && languages[0] === 'auto') {
    return t ? t('common:languageNames.auto') : 'Auto'
  }
  return languages.map(l => l.toUpperCase()).join(', ')
}
