package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// Bounds for the lazy speaker-suggestion path (Power-of-10 rule 2 + input limits).
const (
	// maxPromptUtterances caps how many utterance lines feed the prompt.
	maxPromptUtterances = 5000
	// maxPromptInputRunes caps the total prompt transcript size (cost/latency
	// bound). Lines are otherwise uncapped and the single-speaker fallback sends
	// the entire transcript, so a hard rune budget prevents an unbounded prompt.
	// ~24k runes is enough context to identify speakers from self-introductions
	// without sending the whole transcript to the model.
	maxPromptInputRunes = 24000
	// maxSuggestionEvidence caps stored evidence length (runes).
	maxSuggestionEvidence = 240
	// speakerSuggestionTimeout bounds the synchronous provider call so a stalled
	// provider cannot hold the request for the full server WriteTimeout.
	speakerSuggestionTimeout = 60 * time.Second
	// maxSuggestionAttempts bounds retries of a NON-definitive provider failure
	// (non-block error that consistently fails). The frontend auto-generate effect
	// re-calls the billable model on every editor open while suggestions are not
	// generated, so an always-failing transcript would re-call unbounded. Allow a
	// couple retries to recover from genuinely transient outages (timeout /
	// truncation), then give up by marking generated to cap billable re-calls.
	maxSuggestionAttempts = 3
)

// suggestionDenylist is the set of role-only / honorific-only labels that are
// never accepted as a speaker name. Keys are lowercase, trimmed; lookups
// trim+lowercase the candidate first. Kept package-level so it is testable and
// maintainable. Mirrors the omit list in the provider system instruction.
//
// Matching is intentionally EXACT-MATCH only: it drops standalone honorific or
// role labels (e.g. "Pak", "the witness"). Honorific+name forms like "Pak Budi"
// contain a real personal name and are a legitimate speaker identification, so
// they are deliberately KEPT for the user to curate. Do NOT change this to a
// prefix/contains match — that would over-drop valid named suggestions.
var suggestionDenylist = map[string]struct{}{
	"pak":         {},
	"bu":          {},
	"ibu":         {},
	"bapak":       {},
	"saudara":     {},
	"saksi":       {},
	"the witness": {},
	"witness":     {},
	"interviewer": {},
	"interviewee": {},
	"counsel":     {},
	"the court":   {},
	"court":       {},
	"your honor":  {},
	"host":        {},
	"guest":       {},
	"speaker":     {},
	"unknown":     {},
	"narrator":    {},
	"moderator":   {},
}

// promptUtterance is the minimal utterance shape needed to build the indexed
// prompt. speaker is the numeric diarization index; text is the spoken content.
type promptUtterance struct {
	Speaker int    `json:"speaker"`
	Text    string `json:"text"`
}

// buildSpeakerPromptInput builds a speaker-indexed transcript for the model.
//
// When utterances are present and parseable it emits one "[Speaker N] <text>"
// line per utterance (bounded by maxPromptUtterances) and returns feasible=true.
// When utterances are absent/empty/unparseable it falls back: a single-speaker
// transcript is feasible (the whole text is Speaker 1); a multi-speaker one is
// NOT feasible because there is no reliable index→text mapping.
func buildSpeakerPromptInput(content domain.TranscriptionContent, speakerCount int) (string, bool) {
	lines, ok := buildUtteranceLines(content.Utterances)
	if ok {
		return capRunes(lines, maxPromptInputRunes), true
	}
	if speakerCount == 1 {
		text := strings.TrimSpace(content.FullTranscript)
		if text == "" {
			return "", false
		}
		return capRunes(domain.DefaultSpeakerLabel(0)+" transcript:\n"+text, maxPromptInputRunes), true
	}
	return "", false
}

// buildUtteranceLines parses utterances JSON into indexed lines. Returns ok=false
// when the input is empty, unparseable, or yields no usable lines.
func buildUtteranceLines(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var utterances []promptUtterance
	if err := json.Unmarshal(raw, &utterances); err != nil {
		return "", false
	}
	if len(utterances) == 0 {
		return "", false
	}

	var b strings.Builder
	written := 0
	// Bounded loop: stop after maxPromptUtterances lines (Power-of-10 rule 2).
	for i := 0; i < len(utterances) && written < maxPromptUtterances; i++ {
		text := strings.TrimSpace(utterances[i].Text)
		if text == "" {
			continue
		}
		if written > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte('[')
		b.WriteString(domain.DefaultSpeakerLabel(utterances[i].Speaker))
		b.WriteString("] ")
		b.WriteString(text)
		written++
	}
	if written == 0 {
		return "", false
	}
	return b.String(), true
}

// sanitizeSpeakerSuggestions filters and normalizes raw model output against the
// known speaker count. It drops out-of-range indices, blank/default/denylisted
// names, and blank evidence; trims and length-caps fields; and normalizes
// confidence to high/medium/low. The returned map has at most one entry per
// valid index.
func sanitizeSpeakerSuggestions(raw map[string]domain.SpeakerSuggestion, speakerCount int) map[string]domain.SpeakerSuggestion {
	out := make(map[string]domain.SpeakerSuggestion, len(raw))
	if speakerCount <= 0 {
		return out
	}
	// Bounded: raw is bounded by the provider's response size.
	for key, sug := range raw {
		idx, err := strconv.Atoi(key)
		if err != nil || idx < 0 || idx >= speakerCount {
			continue
		}
		clean, ok := sanitizeOneSuggestion(sug)
		if !ok {
			continue
		}
		out[key] = clean
	}
	return out
}

// sanitizeOneSuggestion cleans a single suggestion. Returns ok=false when the
// entry must be dropped.
func sanitizeOneSuggestion(sug domain.SpeakerSuggestion) (domain.SpeakerSuggestion, bool) {
	// Cap the name FIRST, then run the trim/blank/default-label/denylist checks on
	// the capped value. A >MaxSpeakerNameLength name like "Speaker 1 ..." would
	// otherwise pass IsDefaultSpeakerName on the long form, then truncate to exactly
	// "Speaker 1" and slip through as a valid suggestion.
	name := strings.TrimSpace(capRunes(strings.TrimSpace(sug.Name), domain.MaxSpeakerNameLength))
	evidence := strings.TrimSpace(sug.Evidence)
	if name == "" || evidence == "" {
		return domain.SpeakerSuggestion{}, false
	}
	if domain.IsDefaultSpeakerName(name) {
		return domain.SpeakerSuggestion{}, false
	}
	if _, denied := suggestionDenylist[strings.ToLower(name)]; denied {
		return domain.SpeakerSuggestion{}, false
	}
	return domain.SpeakerSuggestion{
		Name:       name,
		Evidence:   capRunes(evidence, maxSuggestionEvidence),
		Confidence: normalizeConfidence(sug.Confidence),
	}, true
}

// capRunes truncates s to at most limit runes without splitting a rune.
func capRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

// normalizeConfidence maps arbitrary model output to a known level. Anything
// unrecognized degrades to ConfidenceLow.
func normalizeConfidence(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case domain.ConfidenceHigh:
		return domain.ConfidenceHigh
	case domain.ConfidenceMedium:
		return domain.ConfidenceMedium
	default:
		return domain.ConfidenceLow
	}
}

// confidenceRank orders confidence levels for the merge rule.
func confidenceRank(c string) int {
	switch c {
	case domain.ConfidenceHigh:
		return 3
	case domain.ConfidenceMedium:
		return 2
	case domain.ConfidenceLow:
		return 1
	default:
		return 0
	}
}

// mergeSuggestions writes sanitized suggestions into content per the merge rule:
//   - a human-confirmed index (non-empty SpeakerMap entry) is never overwritten;
//   - an existing suggestion of equal-or-higher confidence is kept;
//   - otherwise the new suggestion wins.
//
// content.SuggestedSpeakerMap is initialized before the loop (sized to the
// sanitized input) so the merge body has a non-nil destination to write into.
func mergeSuggestions(content *domain.TranscriptionContent, sanitized map[string]domain.SpeakerSuggestion) {
	if content.SuggestedSpeakerMap == nil {
		content.SuggestedSpeakerMap = make(map[string]domain.SpeakerSuggestion, len(sanitized))
	}
	for index, next := range sanitized {
		if content.SpeakerMap[index] != "" {
			continue // human-confirmed wins
		}
		if existing, ok := content.SuggestedSpeakerMap[index]; ok {
			if confidenceRank(existing.Confidence) >= confidenceRank(next.Confidence) {
				continue
			}
		}
		content.SuggestedSpeakerMap[index] = next
	}
}

// GenerateSpeakerSuggestions lazily produces AI speaker-name suggestions for a
// completed transcription. It is synchronous, idempotent, and never fails the
// transcription: a non-definitive provider error stays retryable but BOUNDED —
// each failure increments a persisted attempt counter and, after
// maxSuggestionAttempts, marks generated to stop unbounded billable re-calls.
// Definitive outcomes mark generated immediately — infeasible input, a
// deterministic safety/content block (domain.ErrContentBlocked), a nil provider
// result, or a successful provider call (even one yielding zero suggestions).
// Privilege media is excluded — it keeps its manual rename gate. The model is
// called at most once, outside the CAS write loop.
func (s *TranscriptionService) GenerateSpeakerSuggestions(ctx context.Context, orgID, transcriptionID string) (*domain.Transcription, error) {
	trans, err := s.GetByID(ctx, orgID, transcriptionID)
	if err != nil {
		return nil, err
	}
	// Guards run cheapest-first: in-memory checks short-circuit before the
	// privilege media-repo lookup. Privilege stays the last guard before
	// generation.
	if trans.Status != domain.TranscriptionStatusCompleted || trans.SpeakerCount <= 0 {
		return trans, nil
	}
	if s.suggestionProvider == nil {
		return trans, nil // feature off; do not mark generated
	}
	// Idempotency: never re-call the provider once generated or already populated.
	if trans.SuggestionsGenerated || len(trans.SuggestedSpeakerMap) > 0 {
		return trans, nil
	}
	if blocked, perr := s.suggestionsBlockedByPrivilege(ctx, trans); perr != nil {
		return nil, perr
	} else if blocked {
		return trans, nil
	}

	input, feasible := buildSpeakerPromptInput(domain.TranscriptionContent{
		FullTranscript: trans.FullTranscript,
		Utterances:     trans.Utterances,
	}, trans.SpeakerCount)
	if !feasible {
		return s.markSuggestionsGenerated(ctx, orgID, transcriptionID)
	}

	// Bound the synchronous provider call: a stalled provider must not hold the
	// request for the full server WriteTimeout. A timeout surfaces as a provider
	// error and degrades gracefully (mark generated, no suggestions, 200).
	callCtx, cancel := context.WithTimeout(ctx, speakerSuggestionTimeout)
	defer cancel()
	result, callErr := s.suggestionProvider.SuggestSpeakers(callCtx, port.SpeakerSuggestionRequest{
		Transcript: input,
		Language:   firstLanguageOrEmpty(trans),
	})
	if callErr != nil {
		// DEFINITIVE vs TRANSIENT failure distinction:
		//
		// A safety/content block (domain.ErrContentBlocked) is DETERMINISTIC — the
		// provider permanently refuses this transcript, common for sensitive
		// legal/notarial content. Retrying re-issues the same blocked, billable call
		// every editor open, forever (unbounded spend). So a block is DEFINITIVE:
		// mark generated to stop re-calling. No suggestions are produced.
		if errors.Is(callErr, domain.ErrContentBlocked) {
			s.logger.Warn("speaker suggestion blocked by provider — marking generated to stop re-calls",
				"transcription_id", transcriptionID, "error", callErr)
			return s.markSuggestionsGenerated(ctx, orgID, transcriptionID)
		}
		// A saturated local model says nothing about this transcript: return
		// without suggestions and without spending one of the bounded attempts,
		// so the next editor open tries again.
		if errors.Is(callErr, domain.ErrModelBusy) {
			s.logger.Info("speaker suggestion skipped: local model busy", "transcription_id", transcriptionID)
			return trans, nil
		}
		// Otherwise the error is NON-DEFINITIVE (e.g. timeout / model unavailable /
		// consistently unparseable output) and stays retryable — but BOUNDED. A
		// one-off Gemini failure at first-open must not permanently disable
		// suggestions, yet a transcript that fails every time must not re-call the
		// billable model on every editor open forever. So record one bounded
		// attempt: it stays retryable until maxSuggestionAttempts is reached, then
		// gives up (marks generated). Always nil error — never fails the request.
		s.logger.Warn("speaker suggestion provider failed",
			"transcription_id", transcriptionID, "error", callErr)
		return s.recordFailedSuggestionAttempt(ctx, orgID, transcriptionID)
	}
	if result == nil {
		// Definitive: the provider returned no result (port contract does not forbid
		// (nil, nil)). Mark generated so a misbehaving provider can't be re-called
		// every open, and avoid dereferencing a nil result below.
		return s.markSuggestionsGenerated(ctx, orgID, transcriptionID)
	}

	sanitized := sanitizeSpeakerSuggestions(result.Suggestions, trans.SpeakerCount)
	updated, err := s.mutateContent(ctx, orgID, transcriptionID, func(content *domain.TranscriptionContent) error {
		content.SuggestionsGenerated = true
		mergeSuggestions(content, sanitized)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("speaker suggestions generated",
		"transcription_id", transcriptionID, "suggestions", len(sanitized))
	return updated, nil
}

// suggestionsBlockedByPrivilege reports whether the transcription is privilege
// media (which keeps its manual rename gate). URL transcriptions have no media
// and are never privilege. Media-not-found is treated as not-blocked.
func (s *TranscriptionService) suggestionsBlockedByPrivilege(ctx context.Context, trans *domain.Transcription) (bool, error) {
	if trans.MediaID == "" {
		return false, nil
	}
	media, err := s.mediaRepo.GetByID(ctx, trans.MediaID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return media.IsPrivilege(), nil
}

// markSuggestionsGenerated flips SuggestionsGenerated without writing any
// suggestions, so a non-feasible or failed transcript is not re-attempted. It
// performs the CAS write and returns the updated transcription.
func (s *TranscriptionService) markSuggestionsGenerated(ctx context.Context, orgID, transcriptionID string) (*domain.Transcription, error) {
	return s.mutateContent(ctx, orgID, transcriptionID, func(content *domain.TranscriptionContent) error {
		content.SuggestionsGenerated = true
		return nil
	})
}

// recordFailedSuggestionAttempt increments the persisted attempt counter for a
// non-definitive provider failure and, once maxSuggestionAttempts is reached,
// marks generated to stop the frontend from re-calling the billable model on
// every editor open. Returns the updated transcription with a NIL error: the
// failure is still graceful and retryable until the cap is hit.
func (s *TranscriptionService) recordFailedSuggestionAttempt(ctx context.Context, orgID, transcriptionID string) (*domain.Transcription, error) {
	return s.mutateContent(ctx, orgID, transcriptionID, func(content *domain.TranscriptionContent) error {
		content.SuggestionAttempts++
		if content.SuggestionAttempts >= maxSuggestionAttempts {
			content.SuggestionsGenerated = true // give up after N failed attempts
		}
		return nil
	})
}

// DismissSpeakerSuggestion removes a single AI suggestion by speaker index. It
// never touches the human-confirmed SpeakerMap and is idempotent: dismissing an
// absent index is a no-op. Org isolation and CAS semantics come from
// mutateContent; ErrConflict propagates unchanged.
func (s *TranscriptionService) DismissSpeakerSuggestion(ctx context.Context, orgID, transcriptionID, index string) error {
	if index == "" {
		return fmt.Errorf("speaker index cannot be empty: %w", domain.ErrInvalidInput)
	}
	if _, err := strconv.Atoi(index); err != nil {
		return fmt.Errorf("speaker index %q is not an integer: %w", index, domain.ErrInvalidInput)
	}
	// Run the same privilege read guard every other transcript path enforces via
	// GetByID. mutateContent only checks org isolation, so calling it directly
	// would let a privilege/legal-hold transcript return 204 instead of 404 —
	// a same-org existence oracle. GetByID returns ErrNotFound for privilege.
	if _, err := s.GetByID(ctx, orgID, transcriptionID); err != nil {
		return err
	}
	if _, err := s.mutateContent(ctx, orgID, transcriptionID, func(content *domain.TranscriptionContent) error {
		delete(content.SuggestedSpeakerMap, index)
		return nil
	}); err != nil {
		return err
	}
	s.logger.Info("speaker suggestion dismissed",
		"transcription_id", transcriptionID, "org_id", orgID)
	return nil
}

// firstLanguageOrEmpty returns the first language hint, or "" when there is none
// or it is the "auto" sentinel. Gladia/URL transcriptions often store "auto",
// which is a meaningless model hint, so it is treated as no hint.
func firstLanguageOrEmpty(trans *domain.Transcription) string {
	if len(trans.Languages) == 0 {
		return ""
	}
	first := strings.TrimSpace(trans.Languages[0])
	if strings.EqualFold(first, "auto") {
		return ""
	}
	return first
}
