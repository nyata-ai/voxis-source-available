package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/domain"
)

// ListTranscriptionSpeakersInput is the input for the list_transcription_speakers tool.
type ListTranscriptionSpeakersInput struct {
	TranscriptionID string `json:"transcription_id" jsonschema:"The UUID of the transcription"`
}

// UpdateSpeakerLabelsInput is the input for the update_speaker_labels tool.
type UpdateSpeakerLabelsInput struct {
	TranscriptionID string            `json:"transcription_id" jsonschema:"The UUID of the transcription"`
	SpeakerMap      map[string]string `json:"speaker_map" jsonschema:"Partial map of speaker indexes to human labels. Omitted speakers keep their current labels."`
}

type mcpSpeakerStat struct {
	Index        int     `json:"index"`
	Label        string  `json:"label"`
	SegmentCount int     `json:"segment_count"`
	FirstStart   float64 `json:"first_start_seconds"`
	LastEnd      float64 `json:"last_end_seconds"`
}

type mcpRawUtterance struct {
	Speaker      json.RawMessage `json:"speaker"`
	StartSeconds float64         `json:"start"`
	EndSeconds   float64         `json:"end"`
}

func handleListTranscriptionSpeakers(
	ctx context.Context,
	deps Dependencies,
	input ListTranscriptionSpeakersInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScope(ctx, "transcription:read"); denied != nil {
		return denied, nil, nil
	}
	if strings.TrimSpace(input.TranscriptionID) == "" {
		return validationError(ctx, fmt.Errorf("transcription_id is required")), nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "list_transcription_speakers", "get transcription", err), nil, nil
	}
	listErr := validateSpeakerListTranscription(trans)
	if listErr != nil {
		return validationError(ctx, listErr), nil, nil
	}

	items, err := speakerStatsFromTranscription(trans)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}

	r, jErr := jsonResult(struct {
		Items []mcpSpeakerStat `json:"items"`
		Count int              `json:"count"`
	}{Items: items, Count: len(items)})
	return r, nil, jErr
}

func validateSpeakerListTranscription(trans *domain.Transcription) error {
	if trans.Status != domain.TranscriptionStatusCompleted {
		return fmt.Errorf("transcription must be completed before listing speakers")
	}
	if !trans.Diarization {
		return fmt.Errorf("diarization is not enabled for this transcription")
	}
	return nil
}

func handleUpdateSpeakerLabels(
	ctx context.Context,
	deps Dependencies,
	input UpdateSpeakerLabelsInput,
) (*mcp.CallToolResult, any, error) {
	if denied := checkScopes(ctx, []string{"transcription:read", "transcription:write"}); denied != nil {
		return denied, nil, nil
	}
	cleaned, err := validateSpeakerMapInput(input)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}

	orgID, err := orgIDFromContext(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	trans, err := deps.TranscriptionService.GetByID(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "update_speaker_labels", "get transcription", err), nil, nil
	}
	patch, err := speakerLabelPatchForUpdate(trans, cleaned)
	if err != nil {
		return validationError(ctx, err), nil, nil
	}
	updateErr := deps.TranscriptionService.UpdateSpeakerLabels(ctx, orgID, input.TranscriptionID, patch)
	if updateErr != nil {
		return toolError(ctx, deps.Logger, "update_speaker_labels", "update speaker labels", updateErr), nil, nil
	}
	trans, err = deps.TranscriptionService.GetByID(ctx, orgID, input.TranscriptionID)
	if err != nil {
		return toolError(ctx, deps.Logger, "update_speaker_labels", "get updated transcription", err), nil, nil
	}

	r, jErr := jsonResult(updatedSpeakerLabelsResult(trans))
	return r, nil, jErr
}

func speakerLabelPatchForUpdate(trans *domain.Transcription, updates map[string]string) (map[string]string, error) {
	if err := validateSpeakerListTranscription(trans); err != nil {
		return nil, err
	}
	indexes, err := speakerIndexesForTranscription(trans)
	if err != nil {
		return nil, err
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf("transcription has no diarized speakers")
	}
	indexSet := make(map[int]bool, len(indexes))
	for _, index := range indexes {
		indexSet[index] = true
	}
	merged := existingSpeakerMapWithDefaults(trans, indexes)
	for key, label := range updates {
		index, _ := parseSpeakerIndex(key)
		if !indexSet[index] {
			return nil, fmt.Errorf("unknown speaker index %d", index)
		}
		merged[strconv.Itoa(index)] = label
	}
	if err := validateUniqueSpeakerLabels(merged); err != nil {
		return nil, err
	}
	return updates, nil
}

func existingSpeakerMapWithDefaults(trans *domain.Transcription, indexes []int) map[string]string {
	merged := make(map[string]string, len(indexes))
	for _, index := range indexes {
		key := strconv.Itoa(index)
		label := strings.TrimSpace(trans.SpeakerMap[key])
		if label == "" {
			label = defaultSpeakerLabel(index)
		}
		merged[key] = label
	}
	return merged
}

func validateUniqueSpeakerLabels(speakerMap map[string]string) error {
	return domain.ValidateUniqueSpeakerLabels(speakerMap)
}

func speakerStatsFromTranscription(trans *domain.Transcription) ([]mcpSpeakerStat, error) {
	utterances, err := parseRawUtterances(trans.Utterances)
	if err != nil {
		return nil, err
	}
	byIndex := make(map[int]*mcpSpeakerStat)
	for _, utterance := range utterances {
		index, ok := domain.SpeakerIndexFromRaw(utterance.Speaker)
		if !ok {
			continue
		}
		label := speakerLabelForIndex(index, trans.SpeakerMap)
		updateSpeakerStat(byIndex, index, label, utterance.StartSeconds, utterance.EndSeconds)
	}
	return sortedSpeakerStats(byIndex), nil
}

func parseRawUtterances(raw json.RawMessage) ([]mcpRawUtterance, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var utterances []mcpRawUtterance
	if err := json.Unmarshal(raw, &utterances); err != nil {
		return nil, fmt.Errorf("parse utterances: %w", err)
	}
	return utterances, nil
}

func updateSpeakerStat(byIndex map[int]*mcpSpeakerStat, index int, label string, start, end float64) {
	stat := byIndex[index]
	if stat == nil {
		stat = &mcpSpeakerStat{Index: index, Label: label, FirstStart: start, LastEnd: end}
		byIndex[index] = stat
	}
	stat.SegmentCount++
	if start < stat.FirstStart {
		stat.FirstStart = start
	}
	if end > stat.LastEnd {
		stat.LastEnd = end
	}
}

func speakerIndexesForTranscription(trans *domain.Transcription) ([]int, error) {
	indexes, err := domain.SpeakerIndexesFromUtterances(trans.Utterances)
	if err != nil {
		return nil, err
	}
	if len(indexes) == 0 && trans.SpeakerCount > 0 {
		indexes = make([]int, 0, trans.SpeakerCount)
		for i := 0; i < trans.SpeakerCount; i++ {
			indexes = append(indexes, i)
		}
	}
	return indexes, nil
}

func parseSpeakerIndex(value string) (int, bool) {
	return domain.SpeakerIndexFromIdentifier(value)
}

func speakerLabelForIndex(index int, speakerMap map[string]string) string {
	key := strconv.Itoa(index)
	if label := strings.TrimSpace(speakerMap[key]); label != "" {
		return label
	}
	return domain.DefaultSpeakerLabel(index)
}

func defaultSpeakerLabel(index int) string {
	return domain.DefaultSpeakerLabel(index)
}

func sortedSpeakerStats(byIndex map[int]*mcpSpeakerStat) []mcpSpeakerStat {
	items := make([]mcpSpeakerStat, 0, len(byIndex))
	for _, stat := range byIndex {
		items = append(items, *stat)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Index < items[j].Index
	})
	return items
}

func validateSpeakerMapInput(input UpdateSpeakerLabelsInput) (map[string]string, error) {
	if strings.TrimSpace(input.TranscriptionID) == "" {
		return nil, fmt.Errorf("transcription_id is required")
	}
	if len(input.SpeakerMap) == 0 {
		return nil, fmt.Errorf("speaker_map is required")
	}
	cleaned := make(map[string]string, len(input.SpeakerMap))
	for key, label := range input.SpeakerMap {
		cleanKey, cleanLabel, err := validateSpeakerMapEntry(key, label)
		if err != nil {
			return nil, err
		}
		if _, exists := cleaned[cleanKey]; exists {
			return nil, fmt.Errorf("duplicate speaker_map key %q", cleanKey)
		}
		cleaned[cleanKey] = cleanLabel
	}
	return cleaned, nil
}

func validateSpeakerMapEntry(key, label string) (cleanKey, cleanLabel string, err error) {
	index, ok := parseSpeakerIndex(key)
	if !ok || index > 99 {
		return "", "", fmt.Errorf("speaker_map key %q must be an integer from 0 to 99", key)
	}
	cleanLabel = strings.TrimSpace(label)
	if err := domain.ValidateSpeakerLabel(index, cleanLabel); err != nil {
		return "", "", err
	}
	return strconv.Itoa(index), cleanLabel, nil
}

func updatedSpeakerLabelsResult(trans *domain.Transcription) any {
	return struct {
		ID           string            `json:"id"`
		SpeakerMap   map[string]string `json:"speaker_map"`
		SpeakerCount int               `json:"speaker_count"`
		Message      string            `json:"message"`
	}{
		ID:           trans.ID,
		SpeakerMap:   trans.SpeakerMap,
		SpeakerCount: trans.SpeakerCount,
		Message:      "Speaker labels updated.",
	}
}
