import type { TranslatedLanguageCode } from './config'

/**
 * Maps an IANA timezone to a shipped UI locale.
 *
 * Timezone stands in for location: it resolves synchronously, needs no network
 * call, and never touches the visitor's IP address. Only the three target
 * markets are mapped — every other zone falls through so the browser language
 * (and then English) decides.
 *
 * Legacy link names are cheap insurance rather than a certainty. V8/ICU
 * canonicalises most CLDR aliases before they reach resolvedOptions()
 * (TZ=Asia/Chongqing is reported as Asia/Shanghai), but Europe/Vaduz and
 * Europe/Busingen are reported verbatim on V8, so those two are required rather
 * than merely insurance. Firefox and Safari could not be tested when this was
 * written. One map entry each.
 *
 * Deliberately unmapped: Asia/Hong_Kong, Asia/Macau, Asia/Taipei. Voxis ships no
 * Traditional Chinese catalog, so the browser language decides for those readers —
 * which in practice still resolves to zh-CN, the closest shipped locale (every zh
 * tag collapses onto it, exactly as i18next's base-language fallback did before
 * this detector existed). Mapping the zones would additionally impose Simplified
 * on a Traditional reader whose browser asked for something else entirely. Also
 * unmapped: UTC, which is what Firefox resistFingerprinting and Tor Browser report.
 */
export const ZONE_LOCALE: Readonly<Partial<Record<string, TranslatedLanguageCode>>> = {
  // Indonesia
  'Asia/Jakarta': 'id',
  'Asia/Pontianak': 'id',
  'Asia/Makassar': 'id',
  'Asia/Ujung_Pandang': 'id',
  'Asia/Jayapura': 'id',
  // Mainland China
  'Asia/Shanghai': 'zh-CN',
  'Asia/Urumqi': 'zh-CN',
  'Asia/Chongqing': 'zh-CN',
  'Asia/Chungking': 'zh-CN',
  'Asia/Harbin': 'zh-CN',
  'Asia/Kashgar': 'zh-CN',
  // Germany, Switzerland, Austria (and Liechtenstein, which is German-speaking)
  'Europe/Berlin': 'de',
  'Europe/Busingen': 'de',
  'Europe/Zurich': 'de',
  'Europe/Vienna': 'de',
  'Europe/Vaduz': 'de',
}

/**
 * The locale for the browser's current timezone, or undefined when the zone is
 * not one of the three target markets. Never throws: a missing signal is a valid
 * answer, and the caller simply falls through to the next detector.
 */
export function localeFromTimeZone(): string | undefined {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
    return zone ? ZONE_LOCALE[zone] : undefined
  } catch {
    // Intl can throw in locked-down or exotic environments.
    return undefined
  }
}
