package speechmatics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/voxis/backend/internal/domain"
)

// voxisToMeliaLanguage maps every code in domain.AllowedLanguages to the
// Speechmatics language-hint code. Melia 1 always runs with language "multi";
// the hints only bias detection.
//
// Only "zh" is renamed: Speechmatics names Mandarin "cmn" (ISO 639-3),
// reserving "yue" for Cantonese. Exposing yue/cmn as user-facing picker
// options is out of scope — that would trigger the four-language user-guide
// obligation — so "zh" maps to Mandarin, which is what the picker means today.
//
// "auto" is absent on purpose: it means "no hints", and the caller
// (BuildTranscriptionRequest) already turns it into an empty Languages slice.
//
// Four codes map to "": jv (Javanese), su (Sundanese), af (Afrikaans) and la
// (Latin) are valid Voxis picker choices but are NOT Speechmatics languages
// (verified against the official Speechmatics Languages page 2026-09-03).
// Forwarding one of them 400s the job submission, which the worker treats as
// a permanent failure — the transcription dies over a hint, not the audio
// itself. MapLanguageHints treats an empty value as "skip this code, submit
// without a hint for it" rather than as a mapping error; DroppedLanguageHints
// tells a caller which of these fired for a given request so it can be
// logged.
//
// This map is exhaustive over domain.AllowedLanguages (a test enforces
// that), so MapLanguageHints can only ever reject a code that did NOT come
// from the picker — in practice SPEECHMATICS_LANGUAGE_HINTS, which is why
// validating it at startup matters.
var voxisToMeliaLanguage = map[string]string{
	"id": "id", "jv": "", "su": "", "en": "en", "ms": "ms", "th": "th", "vi": "vi",
	"zh": "cmn", "ja": "ja", "ko": "ko",
	"es": "es", "de": "de", "ru": "ru", "fr": "fr",
	"ar": "ar", "af": "", "he": "he",
	"tl": "tl", "pl": "pl", "it": "it",
	"sv": "sv", "da": "da", "no": "no", "fi": "fi", "pt": "pt", "la": "",
}

// MapLanguageHints translates Voxis language codes to Melia hints.
//
// An unknown code is a hard error, never a silent drop: dropping it would send
// audio to the provider under different language assumptions than the user
// chose, and the resulting transcript would look plausible while being wrong.
// Because the map above covers every allowed language, the only caller that can
// actually trip this is LoadConfig reading SPEECHMATICS_LANGUAGE_HINTS — a
// deployment typo fails the process instead of every job.
//
// A code Melia has no language for (see voxisToMeliaLanguage) is different: it
// is a KNOWN code that simply contributes no hint, so it is silently skipped
// rather than rejected. Use DroppedLanguageHints to find out which of a
// request's codes were skipped for this reason.
//
// Duplicates collapse; the result is sorted so a request body is deterministic.
func MapLanguageHints(codes []string) ([]string, error) {
	if len(codes) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(codes))
	hints := make([]string, 0, len(codes))
	for _, raw := range codes {
		code := strings.ToLower(strings.TrimSpace(raw))
		if code == "" || code == "auto" {
			continue
		}
		hint, ok := voxisToMeliaLanguage[code]
		if !ok {
			return nil, fmt.Errorf("speechmatics: unsupported language %q: %w", raw, domain.ErrInvalidInput)
		}
		if hint == "" {
			// A known Voxis code with no Melia equivalent (jv/su/af/la); drop
			// it rather than send a hint the provider will reject.
			continue
		}
		if _, dup := seen[hint]; dup {
			continue
		}
		seen[hint] = struct{}{}
		hints = append(hints, hint)
	}
	if len(hints) == 0 {
		return nil, nil
	}
	sort.Strings(hints)
	return hints, nil
}

// DroppedLanguageHints returns the codes in codes that MapLanguageHints would
// silently skip because Melia has no language for them — sorted, deduped and
// lower-cased. An unknown code (one not in domain.AllowedLanguages at all) is
// not included here: that is MapLanguageHints' error case, not a drop.
func DroppedLanguageHints(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(codes))
	dropped := make([]string, 0)
	for _, raw := range codes {
		code := strings.ToLower(strings.TrimSpace(raw))
		hint, ok := voxisToMeliaLanguage[code]
		if !ok || hint != "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		dropped = append(dropped, code)
	}
	if len(dropped) == 0 {
		return nil
	}
	sort.Strings(dropped)
	return dropped
}
