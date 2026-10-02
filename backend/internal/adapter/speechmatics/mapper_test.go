package speechmatics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/voxis/backend/internal/domain"
)

// loadFixture reads a golden json-v2 transcript.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return raw
}

// decodeUtterances unmarshals the emitted utterance array into generic maps so
// tests can assert on which keys are present, not just on their values.
func decodeUtterances(t *testing.T, raw []byte) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestMapTranscript_CodeSwitched(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "code_switched.json"))
	require.NoError(t, err)

	assert.Equal(t, "done", result.Status)
	assert.Equal(t, "Jadi kita review kontraknya. Okay, siap.", result.FullTranscript)
	assert.Equal(t, []string{"en", "id"}, result.Languages)
	assert.Equal(t, 2, result.SpeakerCount)
	assert.Equal(t, 6, result.WordCount)
	assert.InDelta(t, 12.5, result.AudioDuration, 0.001)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 2)

	// Speaker labels map to the integer indices the rest of the pipeline uses.
	assert.EqualValues(t, 0, utterances[0]["speaker"])
	assert.EqualValues(t, 1, utterances[1]["speaker"])

	// Majority word language wins at utterance level (3 id vs 1 en).
	assert.Equal(t, "id", utterances[0]["language"])

	// Punctuation folds into the preceding word rather than becoming a word.
	words := utterances[0]["words"].([]interface{})
	require.Len(t, words, 4)
	assert.Equal(t, "kontraknya.", words[3].(map[string]interface{})["word"])
	assert.Equal(t, "en", words[2].(map[string]interface{})["language"])
}

func TestMapTranscript_OmitsConfidenceEverywhere(t *testing.T) {
	for _, fixture := range []string{"code_switched.json", "no_diarization.json", "multi_speaker.json"} {
		t.Run(fixture, func(t *testing.T) {
			result, err := MapTranscript(loadFixture(t, fixture))
			require.NoError(t, err)

			// Melia returns a confidence, but live testing (2026-08-30) found
			// it pinned at a constant 1.0 across every token — no signal.
			// Publishing that would claim certainty; publishing a Go zero
			// value for an absent field would fabricate a 0.0. The key must
			// be absent entirely.
			assert.NotContains(t, string(result.Utterances), "confidence")

			for _, u := range decodeUtterances(t, result.Utterances) {
				_, hasUtteranceConfidence := u["confidence"]
				assert.False(t, hasUtteranceConfidence)
				for _, w := range u["words"].([]interface{}) {
					_, hasWordConfidence := w.(map[string]interface{})["confidence"]
					assert.False(t, hasWordConfidence)
				}
			}
		})
	}
}

func TestMapTranscript_NoDiarization(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "no_diarization.json"))
	require.NoError(t, err)

	// Without diarization every token carries the same (empty) speaker label,
	// so everything lands on speaker 0 — and, since utterances break on speaker
	// change alone, in a single utterance.
	assert.Equal(t, 1, result.SpeakerCount)
	assert.Equal(t, 4, result.WordCount)
	assert.Equal(t, "Selamat pagi. Semua hadir", result.FullTranscript)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 1)
	assert.EqualValues(t, 0, utterances[0]["speaker"])

	// No job.duration in this fixture: the last token's end time is the fallback.
	assert.InDelta(t, 2.40, result.AudioDuration, 0.001)
}

// A sentence end inside one speaker's run must not start a new utterance, or a
// long answer explodes into dozens of one-line rows in the transcript view, the
// PDF and the summary. The pause and duration rules are inert here — the gaps
// are 0.25s and the whole turn is 2.2s — so this golden is unchanged by them.
func TestMapTranscript_UtterancesAreSpeakerTurnsNotSentences(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "speaker_turns.json"))
	require.NoError(t, err)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 2, "two sentences from one speaker are one utterance")

	// S1's two sentences share an utterance...
	assert.Equal(t, "Selamat pagi. Kita mulai.", utterances[0]["text"])
	assert.EqualValues(t, 0, utterances[0]["speaker"])
	assert.Len(t, utterances[0]["words"].([]interface{}), 4)

	// ...and the speaker change is what splits them.
	assert.Equal(t, "Baik.", utterances[1]["text"])
	assert.EqualValues(t, 1, utterances[1]["speaker"])

	assert.Equal(t, "Selamat pagi. Kita mulai. Baik.", result.FullTranscript)
	assert.Equal(t, 2, result.SpeakerCount)
	assert.Equal(t, 5, result.WordCount)
}

// A single narrator never changes speaker, so speaker change alone cannot
// break the transcript up. Eval run 2 (2026-08-30) mapped a real 24-minute
// monologue into 13 utterances, one of them 928 seconds long — one unreadable
// block in the transcript view, in the PDF and in the summary prompt. A pause
// longer than utteranceBreakPauseSeconds now starts a new utterance.
func TestMapTranscript_PauseBreaksAMonologue(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "monologue_pauses.json"))
	require.NoError(t, err)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 2, "one speaker, split only by the 1.85s pause")

	// The 1.85s gap after "disekap." is the break...
	assert.Equal(t, "Pada tanggal dua belas, korban disekap.", utterances[0]["text"])
	assert.InDelta(t, 0.10, utterances[0]["start"], 0.001)
	assert.InDelta(t, 3.35, utterances[0]["end"], 0.001)

	// ...and the 1.35s gap after "datang." is NOT: sub-threshold pauses and
	// sentence ends both keep a flowing answer in one utterance, which is the
	// whole reason utterances are not split on is_eos.
	assert.Equal(t, "Kemudian polisi datang. Tidak ada saksi.", utterances[1]["text"])
	assert.InDelta(t, 5.20, utterances[1]["start"], 0.001)
	assert.InDelta(t, 9.65, utterances[1]["end"], 0.001)

	// Nothing is lost or duplicated by the split.
	assert.Equal(t, 1, result.SpeakerCount)
	assert.Equal(t, 12, result.WordCount)
	assert.Equal(t,
		"Pada tanggal dua belas, korban disekap. Kemudian polisi datang. Tidak ada saksi.",
		result.FullTranscript)
	for _, u := range utterances {
		assert.EqualValues(t, 0, u["speaker"])
	}
}

// Continuous speech with no pause long enough to break on — a read script, an
// excited speaker — must still be cut into readable blocks. The duration
// ceiling force-breaks at the next word boundary.
func TestMapTranscript_DurationCeilingForcesASplit(t *testing.T) {
	// 500 words at 0.4s each = 200s of unbroken speech, with a full stop every
	// fifth word so the force-break has punctuation to trip over.
	const (
		wordCount    = 500
		wordSeconds  = 0.4
		punctSeconds = 0.02
	)

	var b strings.Builder
	b.WriteString(`{"job":{"id":"j","duration":200},"results":[`)
	for i := 0; i < wordCount; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		start := float64(i) * wordSeconds
		fmt.Fprintf(&b, `{"type":"word","start_time":%.2f,"end_time":%.2f,`+
			`"alternatives":[{"content":"kata","language":"id","speaker":"S1"}]}`,
			start, start+wordSeconds-punctSeconds)
		if i%5 == 4 {
			fmt.Fprintf(&b, `,{"type":"punctuation","attaches_to":"previous",`+
				`"start_time":%.2f,"end_time":%.2f,"is_eos":true,`+
				`"alternatives":[{"content":".","language":"id","speaker":"S1"}]}`,
				start+wordSeconds-punctSeconds, start+wordSeconds)
		}
	}
	b.WriteString(`]}`)

	result, err := MapTranscript([]byte(b.String()))
	require.NoError(t, err)

	utterances := decodeUtterances(t, result.Utterances)
	require.GreaterOrEqual(t, len(utterances), 3, "200s of speech cannot be one utterance")

	seenWords := 0
	for i, u := range utterances {
		span := u["end"].(float64) - u["start"].(float64)
		// "~90s": the break happens at a word boundary, then that word's
		// trailing punctuation is folded in behind it.
		assert.LessOrEqual(t, span, maxUtteranceSeconds+1.0,
			"utterance %d runs %.2fs", i, span)
		// A force-break must never strand a full stop at the head of a block.
		assert.False(t, strings.HasPrefix(u["text"].(string), "."),
			"utterance %d begins with punctuation", i)
		seenWords += len(u["words"].([]interface{}))
	}

	// The split is a regrouping, not a rewrite: every word survives exactly once.
	assert.Equal(t, wordCount, seenWords)
	assert.Equal(t, wordCount, result.WordCount)
	assert.Equal(t, 1, result.SpeakerCount)
}

// The word bound is the last resort for a transcript the time-based rules
// cannot see. Real timings hit the 90s ceiling long before 3000 words, so this
// fixture compresses them: 3005 tokens inside 60s, no gaps, nothing for the
// pause or ceiling rule to fire on. That is the shape a provider payload with
// missing or degenerate timestamps takes, and without the word bound a
// diarization failure over hours of audio would build one unbounded utterance.
func TestMapTranscript_UtteranceWordCapForcesASplit(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"job":{"id":"j","duration":100},"results":[`)
	total := maxUtteranceWords + 5
	for i := 0; i < total; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		start := float64(i) * 0.02
		fmt.Fprintf(&b, `{"type":"word","start_time":%.2f,"end_time":%.2f,`+
			`"alternatives":[{"content":"kata","language":"id","speaker":"S1"}]}`,
			start, start+0.02)
	}
	b.WriteString(`]}`)

	result, err := MapTranscript([]byte(b.String()))
	require.NoError(t, err)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 2)
	assert.Len(t, utterances[0]["words"].([]interface{}), maxUtteranceWords)
	assert.Len(t, utterances[1]["words"].([]interface{}), 5)
	assert.Equal(t, 1, result.SpeakerCount)
	assert.Equal(t, total, result.WordCount)
}

func TestMapTranscript_MultiSpeakerIndicesAreLabelOrdered(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "multi_speaker.json"))
	require.NoError(t, err)

	assert.Equal(t, 4, result.SpeakerCount)

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 4)

	// S3 speaks first but still maps to index 2: indices follow the label
	// number, not first-appearance order, so a speaker map stays stable.
	byText := map[string]int{}
	for _, u := range utterances {
		byText[u["text"].(string)] = int(u["speaker"].(float64))
	}
	assert.Equal(t, 0, byText["Tidak"])   // S1
	assert.Equal(t, 1, byText["Mungkin"]) // S2
	assert.Equal(t, 2, byText["Setuju"])  // S3
	assert.Equal(t, 3, byText["Hmm"])     // UU sorts after the numbered labels
}

// A genuinely silent recording — the job envelope is present and the provider
// reports zero duration — still completes empty.
func TestMapTranscript_LegitimatelyEmptyStillCompletes(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "empty.json"))
	require.NoError(t, err)

	assert.Equal(t, "done", result.Status)
	assert.Empty(t, result.FullTranscript)
	assert.Equal(t, 0, result.SpeakerCount)
	assert.Equal(t, 0, result.WordCount)
	assert.Nil(t, result.Languages)
	// Still a valid JSON array, never null: consumers unmarshal it directly.
	assert.JSONEq(t, `[]`, string(result.Utterances))
	assert.Zero(t, result.AudioDuration)
}

// Zero results for audio the provider says had real duration is the provider's
// honest "no speech" answer, not a broken fetch: live testing (2026-08-30)
// confirmed a silent clip returns status "done", a valid envelope, a real
// duration and results: []. Rejecting it made every silent or music-only upload
// a permanently stuck job. It completes as an empty transcript, with a WARN.
func TestMapTranscript_EmptyResultsForRealDurationCompletesWithAWarning(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	result, err := MapTranscript(loadFixture(t, "empty_with_duration.json"))
	require.NoError(t, err)

	assert.Equal(t, "done", result.Status)
	assert.Empty(t, result.FullTranscript)
	assert.Equal(t, 0, result.WordCount)
	assert.JSONEq(t, `[]`, string(result.Utterances))
	// The duration is still billed: the audio was processed, it just had no
	// speech in it.
	assert.InDelta(t, 4.0, result.AudioDuration, 0.001)

	assert.Contains(t, logs.String(), "no speech")
	assert.Contains(t, logs.String(), "duration_seconds=4")
}

// A body that is not a transcript at all still has to be an error the poller
// can simply retry: completing on it would seal a blank transcript and let the
// poller delete the provider's only copy of the audio.
func TestMapTranscript_StructurallyEmptyIsAnError(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"null body", []byte(`null`)},
		{"empty object", []byte(`{}`)},
		{"wrong keys entirely", []byte(`{"transcript":"halo","segments":[{"text":"halo"}]}`)},
		{"results present but job envelope absent", []byte(`{"results":[]}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MapTranscript(tt.raw)
			require.ErrorIs(t, err, domain.ErrInternal)
			// Never permanent: a retry is the whole point.
			assert.NotErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
}

// job.duration is an INTEGER, floored by the provider (27.5s of audio reports
// 27; a 50.66s file reports 50 — verified live 2026-08-30), while token end
// times are accurate floats. Billing compares the provider duration against the
// ffprobe reservation, so a floored value can never trigger the overage charge
// and silently under-bills. The mapper takes whichever is larger.
func TestMapTranscript_AudioDurationTakesTheLargerOfEnvelopeAndTokens(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{
			// The live-observed shape: floored envelope, accurate last token.
			name: "floored envelope loses to the last token end",
			raw: `{"job":{"id":"j","duration":27},"results":[
				{"type":"word","start_time":26.9,"end_time":27.46,
				 "alternatives":[{"content":"selesai","language":"id","speaker":"S1"}]}
			]}`,
			want: 27.46,
		},
		{
			// Trailing silence after the last word: the envelope is longer and
			// is the honest answer.
			name: "envelope wins over an early last token",
			raw: `{"job":{"id":"j","duration":90},"results":[
				{"type":"word","start_time":1.0,"end_time":1.5,
				 "alternatives":[{"content":"halo","language":"id","speaker":"S1"}]}
			]}`,
			want: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MapTranscript([]byte(tt.raw))
			require.NoError(t, err)
			assert.InDelta(t, tt.want, result.AudioDuration, 0.001)
		})
	}
}

func TestMapTranscript_Malformed(t *testing.T) {
	_, err := MapTranscript(loadFixture(t, "malformed.json"))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInternal)
}

func TestMapTranscript_SkipsEmptyAlternatives(t *testing.T) {
	raw := []byte(`{"job":{"duration":1},"results":[
		{"type":"word","start_time":0,"end_time":1,"alternatives":[]},
		{"type":"word","start_time":0,"end_time":1,"alternatives":[{"content":"","speaker":"S1"}]},
		{"type":"word","start_time":1,"end_time":2,"alternatives":[{"content":"ok","speaker":"S1"}]}
	]}`)

	result, err := MapTranscript(raw)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.FullTranscript)
	assert.Equal(t, 1, result.WordCount)
	// No language tags in this payload: the optional key stays absent.
	assert.NotContains(t, string(result.Utterances), "language")
}

func TestMapTranscript_LeadingPunctuationDoesNotPanic(t *testing.T) {
	// Punctuation with no preceding word must not index into an empty slice.
	raw := []byte(`{"job":{"duration":1},"results":[
		{"type":"punctuation","attaches_to":"previous","start_time":0,"end_time":0.1,
		 "alternatives":[{"content":".","speaker":"S1"}]},
		{"type":"word","start_time":0.2,"end_time":0.6,
		 "alternatives":[{"content":"halo","speaker":"S1"}]}
	]}`)

	result, err := MapTranscript(raw)
	require.NoError(t, err)
	assert.Equal(t, ". halo", result.FullTranscript)
}

// Melia emits one json-v2 result per CHARACTER in Chinese, so a blanket space
// separator renders "你好世界" as "你 好 世 界".
func TestMapTranscript_MandarinIsNotSpaceJoined(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "mandarin_code_switched.json"))
	require.NoError(t, err)

	assert.Contains(t, result.FullTranscript, "你好世界")
	assert.NotContains(t, result.FullTranscript, "你 好")

	utterances := decodeUtterances(t, result.Utterances)
	require.Len(t, utterances, 2, "one utterance per speaker turn")

	// Han runs stay unbroken, and a Latin token embedded in Chinese does not
	// re-introduce spaces on either side of itself.
	assert.Equal(t, "你好世界。我用Python写。", utterances[0]["text"])
	assert.Equal(t, "cmn", utterances[0]["language"], "7 cmn tokens vs 1 en")

	// An all-Latin turn is unaffected: Indonesian and English still get spaces.
	assert.Equal(t, "Jadi kita pakai Zoom.", utterances[1]["text"])
	assert.Equal(t, "id", utterances[1]["language"])

	assert.Equal(t, []string{"cmn", "en", "id"}, result.Languages)
	assert.Equal(t, 2, result.SpeakerCount)
	assert.Equal(t, 12, result.WordCount)
}

// The fallback table treats "ko" as spaced (Korean prose is written WITH
// spaces between eojeol). This fixture's metadata says otherwise for its own
// Korean tokens, and metadata wins.
func TestMapTranscript_MetadataDelimiterGluesKoreanDespiteFallbackTable(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "korean_delimiter_override.json"))
	require.NoError(t, err)

	assert.Equal(t, "안녕하세요세계", result.FullTranscript)
	assert.NotContains(t, result.FullTranscript, "안녕하세요 세계")
}

// The fallback table treats "cmn" as unspaced. This fixture's metadata says
// its Mandarin tokens ARE spaced, and metadata wins over the table.
func TestMapTranscript_MetadataDelimiterSpacesMandarinDespiteFallbackTable(t *testing.T) {
	result, err := MapTranscript(loadFixture(t, "mandarin_delimiter_override.json"))
	require.NoError(t, err)

	assert.Equal(t, "你好 世界", result.FullTranscript)
}

func TestJoinWords_SpacingRules(t *testing.T) {
	word := func(text, language string) mappedWord {
		return mappedWord{Word: text, Language: language}
	}

	tests := []struct {
		name  string
		words []mappedWord
		want  string
	}{
		{"latin keeps spaces", []mappedWord{word("Jadi", "id"), word("kita", "id")}, "Jadi kita"},
		{"mandarin glues", []mappedWord{word("你", "cmn"), word("好", "cmn")}, "你好"},
		{"thai glues", []mappedWord{word("สวัส", "th"), word("ดี", "th")}, "สวัสดี"},
		{"japanese glues", []mappedWord{word("こん", "ja"), word("にちは", "ja")}, "こんにちは"},
		{
			"code-switch boundary follows the CJK side",
			[]mappedWord{word("pakai", "id"), word("合", "cmn"), word("同", "cmn")},
			"pakai合同",
		},
		{
			// Korean is written WITH spaces even though Hangul is in the rune
			// fallback ranges — the language tag is what settles it.
			"korean keeps spaces",
			[]mappedWord{word("안녕하세요", "ko"), word("세계", "ko")},
			"안녕하세요 세계",
		},
		{
			// Untagged tokens fall back to inspecting the boundary runes.
			"untagged han glues",
			[]mappedWord{word("世", ""), word("界", "")},
			"世界",
		},
		{
			"untagged latin keeps spaces",
			[]mappedWord{word("selamat", ""), word("pagi", "")},
			"selamat pagi",
		},
		{"single word", []mappedWord{word("halo", "id")}, "halo"},
		{"no words", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, joinWords(tt.words, nil))
		})
	}
}

// The transcript's own metadata.language_pack_info.per_language_word_delimiters
// is authoritative when present, even where it disagrees with the hardcoded
// noSpaceLanguages fallback table in either direction.
func TestJoinWords_MetadataDelimitersOverrideTheFallbackTable(t *testing.T) {
	word := func(text, language string) mappedWord {
		return mappedWord{Word: text, Language: language}
	}

	tests := []struct {
		name       string
		words      []mappedWord
		delimiters map[string]string
		want       string
	}{
		{
			// The fallback table treats "ko" as spaced (Korean prose is written
			// WITH spaces). Metadata saying otherwise for this transcript wins.
			"metadata says korean has no delimiter: glues despite the fallback table",
			[]mappedWord{word("안녕", "ko"), word("하세요", "ko")},
			map[string]string{"ko": ""},
			"안녕하세요",
		},
		{
			// The fallback table treats "cmn" as unspaced. Metadata saying this
			// transcript's Mandarin IS spaced wins.
			"metadata says mandarin has a delimiter: spaces despite the fallback table",
			[]mappedWord{word("你好", "cmn"), word("世界", "cmn")},
			map[string]string{"cmn": " "},
			"你好 世界",
		},
		{
			// A language absent from the metadata map still falls back to the
			// table, even though the map itself is non-nil.
			"language absent from metadata falls back to the table",
			[]mappedWord{word("你", "cmn"), word("好", "cmn")},
			map[string]string{"ko": ""},
			"你好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, joinWords(tt.words, tt.delimiters))
		})
	}
}

func TestCountSpeakersAndWordsAcceptsMappedUtterances(t *testing.T) {
	// The mapper's output must remain parseable by the shared utterance counter.
	// Existing consumers depend on that normalized result.
	result, err := MapTranscript(loadFixture(t, "code_switched.json"))
	require.NoError(t, err)

	speakers, words := domain.CountSpeakersAndWords(result.Utterances)
	assert.Equal(t, 2, speakers)
	assert.Equal(t, 6, words)
}
