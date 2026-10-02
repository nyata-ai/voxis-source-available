package speechmatics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// maxJSONV2Results bounds the result-token loop. Sixty minutes of dense speech
// is roughly 12k tokens, so this is ~150x headroom while still refusing a
// pathological payload instead of walking it.
const maxJSONV2Results = 2_000_000

// utteranceBreakPauseSeconds is the silence between two consecutive tokens that
// ends an utterance even though the same speaker carries on.
//
// Speaker change alone is not enough on a monologue, and monologues are a real
// Voxis workload. Eval run 2 (2026-08-30) transcribed a 24-minute
// single-narrator investigation recording and got 13 utterances, one of them a
// 928-second turn — a fifteen-minute wall of text in the transcript view, in
// one PDF row, and as one block in the summary prompt. 1.5s is a natural
// speech pause: long enough that normal between-sentence gaps (~0.2-0.5s) do
// not fragment a flowing answer the way breaking on is_eos would, short enough
// that a narrator drawing breath between thoughts starts a new paragraph.
const utteranceBreakPauseSeconds = 1.5

// maxUtteranceSeconds caps how long one utterance may run in wall-clock time.
//
// The pause rule needs a partner: continuous speech with no 1.5s gap (a read
// script, an excited speaker) would still build a single unreadable block. The
// ceiling force-breaks at the next WORD boundary — punctuation is attached to
// the preceding word before any break is considered, so a split never strands a
// comma at the head of an utterance. 90s is roughly a long paragraph of speech
// (~250 words) and well under the 928s turn eval run 2 produced.
const maxUtteranceSeconds = 90

// maxUtteranceWords bounds one utterance in tokens, as a last resort.
//
// With real timestamps the 90s ceiling fires long before 3000 words (90s of
// dense speech is ~400). This bound exists for the degenerate transcript the
// time-based rules cannot see: tokens with absent or zero timings, where every
// gap and every span computes to 0 and neither the pause nor the ceiling can
// ever trigger. Without it, a diarization failure over a three-hour recording
// would build a single unbounded utterance that no PDF row or UI list can
// render. It also keeps the grouping loop's inner growth explicitly bounded
// (Power of 10 rule 2).
const maxUtteranceWords = 3000

// noSpaceLanguages are the Melia language tags whose scripts are written
// without spaces between words. Melia emits one json-v2 result per token, so
// joining these with a space renders Mandarin as "你 好 世 界".
//
// Korean is deliberately absent: Hangul appears in the rune fallback below
// because an untagged Hangul token stream is per-syllable, but Korean prose is
// written WITH spaces between eojeol, and Melia tags those tokens "ko".
//
// This is the FALLBACK table only. The transcript's own
// metadata.language_pack_info.per_language_word_delimiters (verified live
// 2026-08-30) is authoritative when a language is present in it — see
// suppressesSpace — and stays correct as Melia adds languages without this
// table needing an update. A language absent from that metadata (or a
// transcript with no metadata block at all) falls back to this table.
var noSpaceLanguages = map[string]struct{}{
	"cmn": {}, "yue": {}, "zh": {}, "ja": {}, "th": {},
}

// speakerLabelPattern matches Speechmatics' "S<n>" diarization labels. Anything
// else (notably "UU", unknown) is a valid label too and gets its own index.
var speakerLabelPattern = regexp.MustCompile(`^S(\d+)$`)

// jsonV2Response is the subset of the Speechmatics json-v2 transcript we read.
// Job is a pointer so an absent envelope is distinguishable from a present one
// reporting zero — the difference between "silent recording" and "this is not a
// transcript at all".
type jsonV2Response struct {
	Job      *jsonV2Job     `json:"job"`
	Results  []jsonV2Result `json:"results"`
	Metadata jsonV2Metadata `json:"metadata"`
}

// jsonV2Metadata is the subset of the transcript's metadata block we read. A
// transcript with no "metadata" key at all decodes into the zero value: an
// empty (nil) delimiter map, which suppressesSpace treats as "consult the
// fallback table" for every language.
type jsonV2Metadata struct {
	LanguagePackInfo jsonV2LanguagePackInfo `json:"language_pack_info"`
}

// jsonV2LanguagePackInfo carries the provider's authoritative per-language
// word delimiter: an empty string means the language is written without
// spaces between words, any other value means it is.
type jsonV2LanguagePackInfo struct {
	PerLanguageWordDelimiters map[string]string `json:"per_language_word_delimiters"`
}

type jsonV2Job struct {
	ID       string  `json:"id"`
	Duration float64 `json:"duration"`
}

type jsonV2Result struct {
	Type         string              `json:"type"`
	StartTime    float64             `json:"start_time"`
	EndTime      float64             `json:"end_time"`
	AttachesTo   string              `json:"attaches_to"`
	IsEOS        bool                `json:"is_eos"`
	Alternatives []jsonV2Alternative `json:"alternatives"`
}

type jsonV2Alternative struct {
	Content  string `json:"content"`
	Language string `json:"language"`
	Speaker  string `json:"speaker"`
}

// mappedUtterance is the Gladia-compatible utterance we emit.
//
// Two deliberate differences from a Gladia utterance:
//   - No "confidence" key at all. Melia DOES return a confidence on every
//     alternative, but live testing on 2026-08-30 found it pinned at exactly
//     1.0 on all 400+ tokens across 5 jobs — no signal, where Gladia's vary
//     0.18-0.98. Publishing a constant 1.0 would tell a user the model was
//     certain of every word; publishing a Go zero value for an absent field
//     would fabricate a 0.0. Omitting the key is the only honest option, and
//     consumers already treat it as optional. Worth re-checking on difficult
//     audio (noise, heavy code-switching) before concluding the field is
//     permanently useless.
//   - An optional "language" carrying Melia's per-token language tags (the
//     utterance value is the majority language of its words). Gladia already
//     uses a "language" key on utterances, so this is additive, not novel.
type mappedUtterance struct {
	Speaker  int          `json:"speaker"`
	Start    float64      `json:"start"`
	End      float64      `json:"end"`
	Text     string       `json:"text"`
	Language string       `json:"language,omitempty"`
	Words    []mappedWord `json:"words"`
}

type mappedWord struct {
	Word     string  `json:"word"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Language string  `json:"language,omitempty"`
}

// MapTranscript converts a Speechmatics json-v2 transcript into the
// provider-neutral result the transcription services already consume.
func MapTranscript(raw []byte) (*port.TranscriptionResult, error) {
	var payload jsonV2Response
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("speechmatics: decode json-v2 transcript: %w", domain.ErrInternal)
	}
	if len(payload.Results) > maxJSONV2Results {
		return nil, fmt.Errorf("speechmatics: transcript has %d results, exceeding the %d cap: %w",
			len(payload.Results), maxJSONV2Results, domain.ErrInternal)
	}
	if err := rejectStructurallyEmpty(payload); err != nil {
		return nil, err
	}
	warnOnEmptyResults(payload)

	speakerIndex := assignSpeakerIndices(payload.Results)
	delimiters := payload.Metadata.LanguagePackInfo.PerLanguageWordDelimiters
	utterances := buildUtterances(payload.Results, speakerIndex, delimiters)

	utterancesJSON, err := json.Marshal(utterances)
	if err != nil {
		return nil, fmt.Errorf("speechmatics: marshal utterances: %w", err)
	}

	speakerCount, wordCount := domain.CountSpeakersAndWords(utterancesJSON)

	return &port.TranscriptionResult{
		Status:         "done",
		FullTranscript: joinUtteranceText(utterances),
		Utterances:     utterancesJSON,
		Languages:      distinctLanguages(utterances),
		SpeakerCount:   speakerCount,
		WordCount:      wordCount,
		AudioDuration:  audioDuration(payload, utterances),
	}, nil
}

// rejectStructurallyEmpty refuses a transcript body that is not a transcript at
// all, and only that.
//
// A "done" job whose body is null, {}, or shaped differently than we expect
// decodes cleanly into zero results. Completing on that seals an empty
// transcript, marks the transcription complete, and lets the poller delete the
// provider job — which by then holds the only copy of the audio. The presence
// of a job envelope is what separates that case from a real answer, so it is
// the only thing checked here.
//
// The guard deliberately does NOT reject zero results for audio with real
// duration. Live testing on 2026-08-30 confirmed that a silent/no-speech clip
// returns status "done", a valid job envelope, duration 8 and results: [] —
// that IS the provider's honest "no speech" answer, and Gladia completes the
// same clip with an empty transcript. Erroring on it made a silent or
// music-only upload a permanently stuck job that the poller retried forever.
// The caller logs the duration instead, so a genuinely broken fetch that
// somehow carried an envelope is still visible in the logs.
func rejectStructurallyEmpty(payload jsonV2Response) error {
	if len(payload.Results) > 0 || payload.Job != nil {
		return nil
	}
	return fmt.Errorf(
		"speechmatics: transcript body carried no job envelope and no results; "+
			"refusing to complete it as an empty transcript: %w", domain.ErrInternal)
}

// warnOnEmptyResults records that the provider transcribed audio to nothing.
// Legitimate for silence or music, but the one line an operator needs when a
// user reports a blank transcript.
//
// It logs to the default logger rather than the client's: MapTranscript is a
// package-level function with no client, and main.go calls slog.SetDefault, so
// the two are the same handler in production.
func warnOnEmptyResults(payload jsonV2Response) {
	if len(payload.Results) > 0 || payload.Job == nil {
		return
	}
	slog.Default().Warn("provider returned no speech for the submitted audio",
		"speechmatics_job_id", payload.Job.ID,
		"duration_seconds", payload.Job.Duration)
}

// assignSpeakerIndices maps Speechmatics speaker labels to the integer speaker
// indices the rest of the pipeline (and the speaker map) uses. "S1" becomes 0,
// "S2" becomes 1, and any non-conforming label sorts after the numbered ones,
// so the mapping is stable regardless of who speaks first.
func assignSpeakerIndices(results []jsonV2Result) map[string]int {
	labels := make(map[string]struct{})
	for i := 0; i < len(results); i++ {
		if alt, ok := primaryAlternative(results[i]); ok {
			labels[alt.Speaker] = struct{}{}
		}
	}

	ordered := make([]string, 0, len(labels))
	for label := range labels {
		ordered = append(ordered, label)
	}
	sort.Slice(ordered, func(i, j int) bool {
		ni, iNumbered := speakerLabelNumber(ordered[i])
		nj, jNumbered := speakerLabelNumber(ordered[j])
		if iNumbered != jNumbered {
			return iNumbered // numbered labels first
		}
		if iNumbered && ni != nj {
			return ni < nj
		}
		return ordered[i] < ordered[j]
	})

	index := make(map[string]int, len(ordered))
	for i, label := range ordered {
		index[label] = i
	}
	return index
}

// speakerLabelNumber extracts n from an "S<n>" label.
func speakerLabelNumber(label string) (int, bool) {
	m := speakerLabelPattern.FindStringSubmatch(label)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// buildUtterances groups the flat json-v2 token stream into readable turns —
// matching the shape Gladia produces and the shape the summary prompts, PDF
// renderer and UI list were built against.
//
// Sentence ends deliberately do NOT break an utterance. Breaking on is_eos
// turns a five-minute answer into forty single-sentence rows, which bloats the
// transcript view, multiplies PDF rows, and dilutes speaker attribution in a
// long summary. The break signals are speaker change, a pause longer than
// utteranceBreakPauseSeconds, and the maxUtteranceSeconds / maxUtteranceWords
// ceilings — see those constants for why each one exists.
//
// Punctuation attaches to the preceding word rather than becoming a word of its
// own, so word counts stay honest — and, because that happens before any break
// is considered, no break ever lands on a punctuation token.
func buildUtterances(results []jsonV2Result, speakerIndex map[string]int, delimiters map[string]string) []mappedUtterance {
	utterances := make([]mappedUtterance, 0)
	var current *mappedUtterance
	currentLabel := ""

	for i := 0; i < len(results); i++ {
		alt, ok := primaryAlternative(results[i])
		if !ok {
			continue
		}

		isPunctuation := results[i].Type == "punctuation" || results[i].AttachesTo == "previous"
		if isPunctuation && current != nil && len(current.Words) > 0 {
			attachPunctuation(current, results[i], alt)
			continue
		}

		if startsNewUtterance(current, currentLabel, results[i], alt) {
			if current != nil {
				utterances = append(utterances, *current)
			}
			current = &mappedUtterance{
				Speaker: speakerIndex[alt.Speaker],
				Start:   results[i].StartTime,
				Words:   make([]mappedWord, 0, 8),
			}
			currentLabel = alt.Speaker
		}

		current.Words = append(current.Words, mappedWord{
			Word:     alt.Content,
			Start:    results[i].StartTime,
			End:      results[i].EndTime,
			Language: alt.Language,
		})
		current.End = results[i].EndTime
	}

	if current != nil {
		utterances = append(utterances, *current)
	}

	for i := range utterances {
		utterances[i].Text = joinWords(utterances[i].Words, delimiters)
		utterances[i].Language = majorityLanguage(utterances[i].Words)
	}
	return utterances
}

// startsNewUtterance reports whether the token at hand must open a new
// utterance instead of extending the open one. Four independent signals:
//
//   - nothing is open yet;
//   - the speaker changed — the primary, natural boundary;
//   - the speaker paused longer than utteranceBreakPauseSeconds, which is the
//     only natural boundary a single-narrator recording has at all;
//   - the utterance would run past maxUtteranceSeconds or maxUtteranceWords.
//
// Only word tokens reach this. Punctuation is folded into the preceding word by
// the caller before any break is considered, so a break can never leave a comma
// or full stop as the first token of an utterance.
//
// Absent or zero timings make both time-based differences 0, so a transcript
// with no usable timestamps degrades to speaker change plus the word bound
// rather than splitting on noise.
func startsNewUtterance(
	current *mappedUtterance,
	currentLabel string,
	result jsonV2Result,
	alt jsonV2Alternative,
) bool {
	if current == nil {
		return true
	}
	if alt.Speaker != currentLabel {
		return true
	}
	if result.StartTime-current.End > utteranceBreakPauseSeconds {
		return true
	}
	if result.EndTime-current.Start > maxUtteranceSeconds {
		return true
	}
	return len(current.Words) >= maxUtteranceWords
}

// attachPunctuation folds a punctuation token into the preceding word so the
// rendered text reads naturally and len(words) stays a word count.
func attachPunctuation(u *mappedUtterance, result jsonV2Result, alt jsonV2Alternative) {
	last := len(u.Words) - 1
	u.Words[last].Word += alt.Content
	if result.EndTime > u.Words[last].End {
		u.Words[last].End = result.EndTime
	}
	if u.Words[last].End > u.End {
		u.End = u.Words[last].End
	}
}

// primaryAlternative returns the first alternative with non-empty content.
func primaryAlternative(result jsonV2Result) (jsonV2Alternative, bool) {
	if len(result.Alternatives) == 0 {
		return jsonV2Alternative{}, false
	}
	alt := result.Alternatives[0]
	if alt.Content == "" {
		return jsonV2Alternative{}, false
	}
	return alt, true
}

// joinWords renders an utterance's tokens as text. Melia emits one token per
// word in spaced scripts but one token per CHARACTER in Chinese, Japanese and
// Thai, so a blanket space separator renders Mandarin as "你 好 世 界".
//
// delimiters is the transcript's own
// metadata.language_pack_info.per_language_word_delimiters, or nil when the
// transcript carried no metadata block — see suppressesSpace.
func joinWords(words []mappedWord, delimiters map[string]string) string {
	var b strings.Builder
	for i := range words {
		if i > 0 && needsSpaceBetween(words[i-1], words[i], delimiters) {
			b.WriteByte(' ')
		}
		b.WriteString(words[i].Word)
	}
	return b.String()
}

// needsSpaceBetween reports whether two adjacent tokens are separated by a
// space. A space is omitted when EITHER side belongs to a script written
// without them, which also drops the space at a code-switch boundary — the
// CJK typographic convention, and the only rule that keeps a run of Han
// characters unbroken no matter which side of it the Latin text sits on.
func needsSpaceBetween(left, right mappedWord, delimiters map[string]string) bool {
	return !suppressesSpace(left.Language, lastRune(left.Word), delimiters) &&
		!suppressesSpace(right.Language, firstRune(right.Word), delimiters)
}

// suppressesSpace reports whether one token sits in a no-space script.
//
// Three tiers, in order:
//  1. delimiters (the transcript's own per_language_word_delimiters) is
//     authoritative when the token's language is a key in it: an empty-string
//     delimiter means no space, any non-empty delimiter means a space. This
//     stays correct as Melia adds or retunes languages, with no code change
//     here.
//  2. Otherwise, Melia's per-token language tag against the noSpaceLanguages
//     fallback table — still the only signal that distinguishes Korean
//     (Hangul, but written WITH spaces) from a per-character Han stream, and
//     the only thing that keeps a Latin loanword inside a Chinese utterance
//     right.
//  3. An untagged token falls back to inspecting its boundary rune.
func suppressesSpace(language string, boundary rune, delimiters map[string]string) bool {
	if language != "" {
		if delim, ok := delimiters[language]; ok {
			return delim == ""
		}
		_, ok := noSpaceLanguages[language]
		return ok
	}
	return isNoSpaceScript(boundary)
}

// isNoSpaceScript reports whether r belongs to a script written without word
// spaces. Hangul is included here — an untagged Hangul stream from Melia is
// per-syllable — but a token tagged "ko" never reaches this fallback.
func isNoSpaceScript(r rune) bool {
	if r == utf8.RuneError || r == 0 {
		return false
	}
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai)
}

func firstRune(s string) rune {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return 0
	}
	return r
}

func lastRune(s string) rune {
	r, size := utf8.DecodeLastRuneInString(s)
	if size == 0 {
		return 0
	}
	return r
}

func joinUtteranceText(utterances []mappedUtterance) string {
	parts := make([]string, 0, len(utterances))
	for _, u := range utterances {
		if text := strings.TrimSpace(u.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// majorityLanguage returns the most common word language in an utterance, with
// ties broken alphabetically for determinism. Empty when no word is tagged.
func majorityLanguage(words []mappedWord) string {
	counts := make(map[string]int, 4)
	for _, w := range words {
		if w.Language != "" {
			counts[w.Language]++
		}
	}
	best := ""
	bestCount := 0
	for lang, n := range counts {
		if n > bestCount || (n == bestCount && lang < best) {
			best, bestCount = lang, n
		}
	}
	return best
}

// distinctLanguages returns every language observed at word level, sorted.
func distinctLanguages(utterances []mappedUtterance) []string {
	seen := make(map[string]struct{}, 4)
	for _, u := range utterances {
		for _, w := range u.Words {
			if w.Language != "" {
				seen[w.Language] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	langs := make([]string, 0, len(seen))
	for lang := range seen {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// audioDuration takes the LARGER of the job's reported duration and the last
// token's end time.
//
// job.duration is an integer, floored: 27.5s of audio reports 27, and a 50.66s
// file reports 50 (verified live 2026-08-30). Token end_time values are floats
// and accurate. Taking the max matters for money: the media path charges the
// difference when the provider-measured duration exceeds the ffprobe-based
// reservation, and a floored integer can never exceed it, so a truncating
// duration silently under-bills every job by up to a second. It also keeps a
// transcript with no job metadata at all billing off its own tokens.
func audioDuration(payload jsonV2Response, utterances []mappedUtterance) float64 {
	longest := 0.0
	for _, u := range utterances {
		if u.End > longest {
			longest = u.End
		}
	}
	if payload.Job != nil && payload.Job.Duration > longest {
		longest = payload.Job.Duration
	}
	return longest
}
