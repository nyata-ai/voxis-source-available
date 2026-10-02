// Package vocab serves the built-in custom-vocabulary packs offered at
// transcription submission.
//
// Packs are static, public content embedded in the binary: only pack *ids* are
// persisted on a transcription row, so nothing client-confidential is stored in
// plaintext. Resolution (ids -> terms) happens in the worker, immediately
// before the provider request is built.
//
// Authoring rules and review cadence live in README.md next to this file.
package vocab

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/voxis/backend/internal/port"
)

//go:embed packs/*.json
var packFS embed.FS

// Request-level caps. These are ours, not the provider's: the provider
// documents no hard limit, and an unbounded list is a cost and latency risk.
const (
	// MaxPacksPerRequest bounds how many packs one submission may select.
	MaxPacksPerRequest = 6
	// MaxTermsPerRequest bounds the merged term list sent to the provider.
	MaxTermsPerRequest = 500
)

// Per-term caps enforced at load time.
const (
	maxValueRunes         = 64
	maxPronunciations     = 5
	maxPronunciationRunes = 64
	maxTermsPerPack       = 1000
	maxPackFiles          = 32
)

// rawPack mirrors one embedded pack file.
type rawPack struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Terms       []rawTerm `json:"terms"`
}

// rawTerm mirrors one entry in a pack file. Gloss is maintenance-only
// documentation and is never sent to the transcription provider.
type rawTerm struct {
	Value          string   `json:"value"`
	Pronunciations []string `json:"pronunciations"`
	Intensity      float64  `json:"intensity"`
	Language       string   `json:"language"`
	Gloss          string   `json:"gloss"`
}

// Loaded state. Populated once at init and read-only afterwards.
var (
	packTerms  map[string][]port.VocabTerm
	packIDs    []string
	loadIssues []string
)

func init() {
	packTerms, packIDs, loadIssues = loadPacks()
}

// LoadIssues reports packs or terms rejected while loading the embedded files.
// A healthy build has none; the package test asserts that. Exposed rather than
// panicking so a malformed pack degrades this optional feature instead of
// preventing the server from starting.
func LoadIssues() []string {
	out := make([]string, len(loadIssues))
	copy(out, loadIssues)
	return out
}

// ValidIDs returns the ids of every loaded pack, in canonical (sorted) order.
func ValidIDs() []string {
	out := make([]string, len(packIDs))
	copy(out, packIDs)
	return out
}

// IsValidID reports whether id names a loaded pack.
func IsValidID(id string) bool {
	_, ok := packTerms[id]
	return ok
}

// Terms resolves pack ids to a merged, deduplicated term list.
//
// Order is canonical pack order then in-pack order, so the result does not
// depend on the order the caller listed the ids. Duplicate ids are ignored,
// duplicate term values collapse to their first occurrence, and the merged list
// is truncated to MaxTermsPerRequest. An empty selection yields no terms.
func Terms(ids []string) ([]port.VocabTerm, error) {
	selected, err := resolveIDs(ids)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, nil
	}

	terms := make([]port.VocabTerm, 0, MaxTermsPerRequest)
	seen := make(map[string]bool, MaxTermsPerRequest)
	truncated := false
	for _, id := range selected {
		for _, term := range packTerms[id] {
			if seen[term.Value] {
				continue
			}
			if len(terms) >= MaxTermsPerRequest {
				truncated = true
				break
			}
			seen[term.Value] = true
			terms = append(terms, cloneTerm(term))
		}
	}
	if truncated {
		slog.Default().Warn("custom vocabulary truncated",
			"limit", MaxTermsPerRequest, "packs", selected)
	}
	return terms, nil
}

// resolveIDs validates the requested ids and returns the matching pack ids in
// canonical order.
func resolveIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > MaxPacksPerRequest {
		return nil, fmt.Errorf("at most %d vocabulary packs allowed, got %d", MaxPacksPerRequest, len(ids))
	}

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if !IsValidID(trimmed) {
			return nil, fmt.Errorf("unknown vocabulary pack %q (valid: %s)", truncateForError(id), strings.Join(packIDs, ", "))
		}
		wanted[trimmed] = true
	}

	selected := make([]string, 0, len(wanted))
	for _, id := range packIDs {
		if wanted[id] {
			selected = append(selected, id)
		}
	}
	return selected, nil
}

// maxReflectedErrorRunes bounds how much caller-supplied text an error message
// may echo back. An unbounded echo lets a caller inflate the error (and the log
// line carrying it) with whatever it sent.
const maxReflectedErrorRunes = 64

// truncateForError bounds a caller-supplied value about to be reflected in an
// error message.
func truncateForError(value string) string {
	if utf8.RuneCountInString(value) <= maxReflectedErrorRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxReflectedErrorRunes]) + "…"
}

// cloneTerm copies a term so callers cannot mutate the shared loaded state.
func cloneTerm(t port.VocabTerm) port.VocabTerm {
	out := t
	if len(t.Pronunciations) > 0 {
		out.Pronunciations = make([]string, len(t.Pronunciations))
		copy(out.Pronunciations, t.Pronunciations)
	}
	return out
}

// loadPacks reads and validates every embedded pack file.
func loadPacks() (packs map[string][]port.VocabTerm, ids, issues []string) {
	packs = make(map[string][]port.VocabTerm)

	entries, err := packFS.ReadDir("packs")
	if err != nil {
		return packs, ids, append(issues, fmt.Sprintf("read packs dir: %v", err))
	}
	if len(entries) > maxPackFiles {
		entries = entries[:maxPackFiles]
		issues = append(issues, fmt.Sprintf("more than %d pack files; extras ignored", maxPackFiles))
	}

	for _, entry := range entries {
		name := path.Join("packs", entry.Name())
		data, readErr := packFS.ReadFile(name)
		if readErr != nil {
			issues = append(issues, fmt.Sprintf("%s: %v", name, readErr))
			continue
		}
		id, packed, packIssues := decodePack(name, data)
		issues = append(issues, packIssues...)
		if id == "" {
			continue
		}
		if _, dup := packs[id]; dup {
			issues = append(issues, fmt.Sprintf("%s: duplicate pack id %q", name, id))
			continue
		}
		packs[id] = packed
		ids = append(ids, id)
	}

	sort.Strings(ids)
	return packs, ids, issues
}

// decodePack parses one pack file. A pack with an unusable header is rejected
// wholesale (empty id returned); individual bad terms are dropped and reported.
func decodePack(name string, data []byte) (id string, terms []port.VocabTerm, issues []string) {
	var raw rawPack
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", nil, []string{fmt.Sprintf("%s: %v", name, err)}
	}

	id = strings.TrimSpace(raw.ID)
	if id == "" {
		return "", nil, []string{fmt.Sprintf("%s: missing pack id", name)}
	}
	if len(raw.Terms) == 0 {
		return "", nil, []string{fmt.Sprintf("%s: pack %q has no terms", name, id)}
	}
	if len(raw.Terms) > maxTermsPerPack {
		return "", nil, []string{fmt.Sprintf("%s: pack %q exceeds %d terms", name, id, maxTermsPerPack)}
	}

	out := make([]port.VocabTerm, 0, len(raw.Terms))
	seen := make(map[string]bool, len(raw.Terms))
	for i, rt := range raw.Terms {
		term, err := normalizeTerm(rt)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: term %d: %v", name, i, err))
			continue
		}
		if seen[term.Value] {
			issues = append(issues, fmt.Sprintf("%s: duplicate term %q", name, term.Value))
			continue
		}
		seen[term.Value] = true
		out = append(out, term)
	}
	return id, out, issues
}

// normalizeTerm trims, validates and clamps one authored term.
func normalizeTerm(rt rawTerm) (port.VocabTerm, error) {
	value := strings.TrimSpace(rt.Value)
	if value == "" {
		return port.VocabTerm{}, fmt.Errorf("empty value")
	}
	if utf8.RuneCountInString(value) > maxValueRunes {
		return port.VocabTerm{}, fmt.Errorf("value %q exceeds %d runes", value, maxValueRunes)
	}
	if len(rt.Pronunciations) > maxPronunciations {
		return port.VocabTerm{}, fmt.Errorf("value %q has more than %d pronunciations", value, maxPronunciations)
	}

	var pronunciations []string
	for _, p := range rt.Pronunciations {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return port.VocabTerm{}, fmt.Errorf("value %q has an empty pronunciation", value)
		}
		if utf8.RuneCountInString(trimmed) > maxPronunciationRunes {
			return port.VocabTerm{}, fmt.Errorf("value %q has a pronunciation over %d runes", value, maxPronunciationRunes)
		}
		pronunciations = append(pronunciations, trimmed)
	}

	intensity := rt.Intensity
	if intensity < 0 {
		intensity = 0
	}
	if intensity > 1 {
		intensity = 1
	}

	return port.VocabTerm{
		Value:          value,
		Pronunciations: pronunciations,
		Intensity:      intensity,
		Language:       strings.ToLower(strings.TrimSpace(rt.Language)),
	}, nil
}
