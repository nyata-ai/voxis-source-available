package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// TranscriptionService orchestrates creating transcriptions, processing
// results from the transcription provider, and decrypting content on read.
type TranscriptionService struct {
	transRepo   port.TranscriptionRepository
	mediaRepo   port.MediaRepository
	summaryRepo port.SummaryRepository
	envelope    *crypto.EnvelopeService
	logger      *slog.Logger

	// Optional: for manual summary generation (GenerateAllSummaries).
	summarySvc             *SummaryService
	summaryJobInserter     port.SummaryJobInserter
	userRepo               port.UserRepository
	summaryProfilesEnabled bool

	// Optional: AI speaker-name suggestion provider. When nil the lazy
	// GenerateSpeakerSuggestions path is a no-op (feature off).
	suggestionProvider port.SpeakerSuggestionProvider
}

// TranscriptionServiceOption configures optional behavior on TranscriptionService.
type TranscriptionServiceOption func(*TranscriptionService)

// WithSummaryGeneration wires the summary service and job inserter used by the
// manual GenerateAllSummaries fan-out. It does not auto-run on completion.
func WithSummaryGeneration(svc *SummaryService, inserter port.SummaryJobInserter) TranscriptionServiceOption {
	return func(s *TranscriptionService) {
		s.summarySvc = svc
		s.summaryJobInserter = inserter
	}
}

// WithSummaryPreferences wires the user repository consulted for per-user
// summary preferences (e.g. high-stakes mode) when generating summaries.
func WithSummaryPreferences(userRepo port.UserRepository) TranscriptionServiceOption {
	return func(s *TranscriptionService) {
		s.userRepo = userRepo
	}
}

// WithSummaryProfilesEnabled controls whether GenerateAllSummaries may use the
// initiating user's stored profile rather than General Professional.
func WithSummaryProfilesEnabled(enabled bool) TranscriptionServiceOption {
	return func(s *TranscriptionService) {
		s.summaryProfilesEnabled = enabled
	}
}

// WithSpeakerSuggestionProvider wires the AI provider used by the lazy
// GenerateSpeakerSuggestions path. When the provider is nil the feature is off.
func WithSpeakerSuggestionProvider(p port.SpeakerSuggestionProvider) TranscriptionServiceOption {
	return func(s *TranscriptionService) {
		s.suggestionProvider = p
	}
}

// NewTranscriptionService creates a new TranscriptionService.
func NewTranscriptionService(
	transRepo port.TranscriptionRepository,
	mediaRepo port.MediaRepository,
	summaryRepo port.SummaryRepository,
	envelope *crypto.EnvelopeService,
	logger *slog.Logger,
	opts ...TranscriptionServiceOption,
) *TranscriptionService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &TranscriptionService{
		transRepo:   transRepo,
		mediaRepo:   mediaRepo,
		summaryRepo: summaryRepo,
		envelope:    envelope,
		logger:      logger,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// ApplyOptions applies additional options after construction.
// Useful when dependencies are resolved in multiple phases (e.g., job inserter
// created after River client initialization).
func (s *TranscriptionService) ApplyOptions(opts ...TranscriptionServiceOption) {
	for _, opt := range opts {
		opt(s)
	}
}

// SubmissionOptions carries the optional per-submission tuning knobs that are
// not part of the core create signature. It exists because both create methods
// already end in a variadic, so no further positional parameter can be added
// without touching every caller.
type SubmissionOptions struct {
	// VocabularyPacks holds built-in custom-vocabulary pack ids; ids are
	// validated at the API boundary, which owns the pack registry.
	VocabularyPacks []string
	// ExpectedSpeakers is the user-declared speaker count, 0 for auto-detect.
	// Validated by domain.ValidateExpectedSpeakers.
	ExpectedSpeakers int
}

// Create creates a new transcription for a media file.
// vocabularyPacks is variadic so the existing call sites that do not select
// custom vocabulary stay unchanged; ids are validated at the API boundary.
func (s *TranscriptionService) Create(ctx context.Context, orgID, mediaID string, languages []string, diarization, enhanceAudio bool, vocabularyPacks ...string) (*domain.Transcription, error) {
	return s.CreateWithOptions(ctx, orgID, mediaID, languages, diarization, enhanceAudio,
		SubmissionOptions{VocabularyPacks: vocabularyPacks})
}

// CreateWithOptions creates a new transcription for a media file.
// Validates: media exists, belongs to org, is in "ready" status.
// Checks for existing active transcription (ErrConflict).
func (s *TranscriptionService) CreateWithOptions(
	ctx context.Context, orgID, mediaID string,
	languages []string, diarization, enhanceAudio bool, opts SubmissionOptions,
) (*domain.Transcription, error) {
	// 1. Validate media exists.
	media, err := s.mediaRepo.GetByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	// 2. Check org isolation.
	if media.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// 3. Check media is ready.
	if media.Status != domain.MediaStatusReady {
		return nil, fmt.Errorf("media is not ready for transcription (status=%s): %w", media.Status, domain.ErrInvalidInput)
	}
	if media.AudioDeletedAt != nil {
		return nil, domain.ErrAudioUnavailable
	}

	// 4. Check for existing active transcription.
	_, err = s.transRepo.GetByMediaID(ctx, mediaID)
	if err == nil {
		// Found an active (non-failed/deleted) transcription.
		return nil, fmt.Errorf("an active transcription already exists for this media: %w", domain.ErrConflict)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		// Unexpected error from repository.
		return nil, fmt.Errorf("check existing transcription: %w", err)
	}

	// 6. Create domain entity.
	trans, err := domain.NewTranscription(orgID, mediaID, languages, diarization, enhanceAudio, opts.VocabularyPacks...)
	if err != nil {
		return nil, err
	}
	if setErr := trans.SetExpectedSpeakers(opts.ExpectedSpeakers); setErr != nil {
		return nil, setErr
	}

	// 7. Persist.
	if err := s.transRepo.Create(ctx, trans); err != nil {
		return nil, fmt.Errorf("persist transcription: %w", err)
	}

	s.logger.Info("transcription created",
		"transcription_id", trans.ID,
		"media_id", mediaID,
		"org_id", orgID,
		"languages", languages,
	)

	return trans, nil
}

// GenerateAllSummaries creates all four summary types for a completed
// transcription and enqueues their jobs. Source-type-agnostic: any per-source
// restriction (e.g. url-only) is the caller's responsibility. Validates org
// ownership and completed status, then runs the idempotent create-four logic.
// Returns domain.ErrNotFound for an unknown transcription or org mismatch, and
// a domain.ErrConflict-wrapped error when the transcription is not completed.
func (s *TranscriptionService) GenerateAllSummaries(ctx context.Context, orgID, transcriptionID string) error {
	return s.generateAllSummaries(ctx, orgID, transcriptionID, "", "", false)
}

// GenerateAllSummariesWithProfile creates summaries using the requesting user's selected profile.
func (s *TranscriptionService) GenerateAllSummariesWithProfile(
	ctx context.Context,
	orgID, transcriptionID, subject, summaryProfile string,
) error {
	return s.generateAllSummaries(ctx, orgID, transcriptionID, subject, summaryProfile, true)
}

func (s *TranscriptionService) generateAllSummaries(
	ctx context.Context,
	orgID, transcriptionID, subject, summaryProfile string,
	profileProvided bool,
) error {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return err
	}
	if trans.OrganizationID != orgID {
		return domain.ErrNotFound
	}
	if trans.Status != domain.TranscriptionStatusCompleted {
		return fmt.Errorf("transcription is not completed (status=%s): %w", trans.Status, domain.ErrConflict)
	}
	prefUserID := subject
	if !profileProvided {
		summaryProfile, err = ResolveSummaryProfile(ctx, s.userRepo, prefUserID, s.summaryProfilesEnabled)
		if err != nil {
			return err
		}
	}
	return s.createAllSummaries(ctx, trans, prefUserID, summaryProfile)
}

// SummaryFanOutError reports that a generate-all run got past every load-phase
// check — the transcription exists, is owned by the org, is completed and is
// readable — but one or more of the per-type creations or enqueues failed.
//
// It deliberately does not implement Unwrap. The joined per-type errors may
// carry domain sentinels (ErrConflict from a racing create, ErrNotFound from a
// vanished row) that describe a single summary type, not the transcription, and
// letting errors.Is see them makes the HTTP layer answer a mostly-successful
// fan-out with a flat 404 or 409 that is wrong on both counts.
type SummaryFanOutError struct {
	TranscriptionID string
	Err             error
}

func (e *SummaryFanOutError) Error() string {
	return fmt.Sprintf("generate summaries for transcription %s: %v", e.TranscriptionID, e.Err)
}

// createAllSummaries creates 4 summary types and enqueues jobs for a completed
// transcription. Idempotent: skips summary types that already exist and
// re-enqueues any that are still pending. prefUserID is the user whose
// high-stakes preference applies (see generateAllSummaries).
//
// Contract with the reader UI: BriefingPane's "Generate all" button is disabled
// once every type has a row, because this function skips existing non-pending
// rows and the button would otherwise do nothing. Relaxing that skip (e.g. to
// regenerate failed rows) requires relaxing BriefingPane's `hasGeneratable`
// in the same change.
//
// A failure after the load phase is returned as *SummaryFanOutError so callers
// can tell "nothing about this transcription is wrong, some types failed" from
// the load-phase refusals generateAllSummaries returns directly.
func (s *TranscriptionService) createAllSummaries(
	ctx context.Context,
	trans *domain.Transcription,
	prefUserID string,
	summaryProfile string,
) error {
	if s.summarySvc == nil || s.summaryJobInserter == nil {
		s.logger.Warn("generate summaries skipped: summary service or job inserter not configured",
			"transcription_id", trans.ID)
		return nil
	}

	summaryTypes := domain.DefaultSummaryTypes
	highStakes, err := ResolveHighStakesPreference(ctx, s.userRepo, prefUserID)
	if err != nil {
		return err
	}
	if !s.summarySvc.HighStakesEnabled() {
		highStakes = false
	}

	var errs []error
	for _, st := range summaryTypes {
		summary, err := s.summarySvc.CreateWithProfile(ctx, trans.OrganizationID, trans.ID, st, highStakes, summaryProfile)
		if err != nil {
			if errors.Is(err, domain.ErrConflict) {
				if enqErr := s.enqueueExistingPendingSummary(ctx, trans, st); enqErr != nil {
					errs = append(errs, enqErr)
				}
				continue
			}
			s.logger.Error("generate summaries: failed to create summary",
				"transcription_id", trans.ID, "type", st, "error", err)
			errs = append(errs, fmt.Errorf("create %s summary: %w", st, err))
			continue
		}

		if insertErr := s.summaryJobInserter.InsertSummarizeJob(
			ctx, summary.ID, trans.ID, trans.OrganizationID,
		); insertErr != nil {
			s.logger.Error("generate summaries: failed to enqueue summary job",
				"summary_id", summary.ID, "type", st, "error", insertErr)
			if deleteErr := s.summarySvc.Delete(ctx, trans.OrganizationID, summary.ID); deleteErr != nil {
				s.logger.Error("generate summaries: failed to clean up orphaned summary",
					"summary_id", summary.ID, "type", st, "error", deleteErr)
			}
			errs = append(errs, fmt.Errorf("enqueue %s summary: %w", st, insertErr))
		}
	}
	if len(errs) > 0 {
		return &SummaryFanOutError{TranscriptionID: trans.ID, Err: errors.Join(errs...)}
	}
	return nil
}

func (s *TranscriptionService) enqueueExistingPendingSummary(ctx context.Context, trans *domain.Transcription, summaryType string) error {
	summaries, err := s.summarySvc.ListByTranscription(ctx, trans.OrganizationID, trans.ID)
	if err != nil {
		s.logger.Error("generate summaries: failed to inspect existing summaries",
			"transcription_id", trans.ID, "type", summaryType, "error", err)
		return fmt.Errorf("inspect %s summary: %w", summaryType, err)
	}

	for _, summary := range summaries {
		if summary.SummaryType != summaryType {
			continue
		}
		if summary.Status != domain.SummaryStatusPending {
			s.logger.Info("generate summaries: summary already exists, skipping",
				"transcription_id", trans.ID, "type", summaryType, "status", summary.Status)
			return nil
		}
		if err := s.summaryJobInserter.InsertSummarizeJob(ctx, summary.ID, trans.ID, trans.OrganizationID); err != nil {
			s.logger.Error("generate summaries: failed to re-enqueue pending summary",
				"summary_id", summary.ID, "type", summaryType, "error", err)
			return fmt.Errorf("re-enqueue %s summary: %w", summaryType, err)
		}
		s.logger.Info("generate summaries: re-enqueued pending summary",
			"summary_id", summary.ID, "type", summaryType)
		return nil
	}

	return fmt.Errorf("active %s summary conflicted but could not be found", summaryType)
}

// ProcessResult processes a completed transcription result from the provider.
// Encrypts the content JSON using EnvelopeService.SealField.
// Idempotent: returns nil if transcription is already completed.
func (s *TranscriptionService) ProcessResult(ctx context.Context, transcriptionID string, result *port.TranscriptionResult) error {
	// 1. Load transcription.
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return fmt.Errorf("load transcription: %w", err)
	}

	// 2. Idempotent: if already completed, return nil.
	// Also reject if failed or deleted — stale callbacks should not resurrect them.
	switch trans.Status {
	case domain.TranscriptionStatusCompleted:
		s.logger.Info("transcription already completed, skipping",
			"transcription_id", transcriptionID,
		)
		return nil
	case domain.TranscriptionStatusFailed, domain.TranscriptionStatusDeleted:
		s.logger.Info("transcription in terminal state, ignoring result",
			"transcription_id", transcriptionID,
			"status", trans.Status,
		)
		return nil
	}

	// 3. Build content JSON.
	content := domain.TranscriptionContent{
		FullTranscript: result.FullTranscript,
		Utterances:     result.Utterances,
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal transcription content: %w", err)
	}

	// 4. Encrypt content via envelope service.
	sealed, err := s.envelope.SealField(ctx, trans.OrganizationID, contentJSON)
	if err != nil {
		return fmt.Errorf("encrypt transcription content: %w", err)
	}

	// 5. Store encrypted content on entity.
	trans.ContentEncrypted = sealed.Ciphertext
	trans.ContentNonce = sealed.Nonce
	trans.WrappedDEK = sealed.WrappedDEK
	trans.WrappingNonce = sealed.WrappingNonce

	// 6. Mark as completed.
	trans.SetCompleted(result.SpeakerCount, result.WordCount, result.AudioDuration)

	// 7. Persist.
	if err := s.persistCompletion(ctx, trans); err != nil {
		return fmt.Errorf("update transcription: %w", err)
	}

	s.logger.Info("transcription result processed",
		"transcription_id", transcriptionID,
		"word_count", result.WordCount,
		"speaker_count", result.SpeakerCount,
		"duration_seconds", result.AudioDuration,
	)

	return nil
}

func (s *TranscriptionService) persistCompletion(ctx context.Context, trans *domain.Transcription) error {
	return s.transRepo.Update(ctx, trans)
}

// HandleFailure marks a transcription as failed with an error message.
// Idempotent: returns nil if already in a terminal state (completed/failed/deleted).
func (s *TranscriptionService) HandleFailure(ctx context.Context, transcriptionID, errMsg string) error {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return fmt.Errorf("load transcription: %w", err)
	}

	// Guard: don't overwrite terminal states with failure.
	switch trans.Status {
	case domain.TranscriptionStatusCompleted:
		s.logger.Info("transcription already completed, ignoring failure callback",
			"transcription_id", transcriptionID,
		)
		return nil
	case domain.TranscriptionStatusFailed:
		s.logger.Info("transcription already failed, skipping",
			"transcription_id", transcriptionID,
		)
		return nil
	case domain.TranscriptionStatusDeleted:
		s.logger.Info("transcription deleted, ignoring failure callback",
			"transcription_id", transcriptionID,
		)
		return nil
	}

	trans.SetFailed(errMsg)

	if err := s.transRepo.Update(ctx, trans); err != nil {
		return fmt.Errorf("update transcription: %w", err)
	}

	s.logger.Warn("transcription failed",
		"transcription_id", transcriptionID,
		"error_message", errMsg,
	)

	return nil
}

// GetByID retrieves a transcription, enforcing org isolation.
// Decrypts content via EnvelopeService.OpenField if completed.
// Also loads media filename for API responses.
func (s *TranscriptionService) GetByID(ctx context.Context, orgID, transcriptionID string) (*domain.Transcription, error) {
	// 1. Load transcription.
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return nil, err
	}

	// 2. Org isolation.
	if trans.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// 3. If deleted, return not found.
	if trans.Status == domain.TranscriptionStatusDeleted {
		return nil, domain.ErrNotFound
	}

	// 4. Decrypt content if completed and has encrypted data.
	if err := s.decryptCompletedContent(ctx, orgID, trans); err != nil {
		return nil, err
	}

	// 5. Load media metadata for the response.
	if err := s.loadSourceMetadata(ctx, trans); err != nil {
		return nil, err
	}

	return trans, nil
}

// ProbeAccess answers "may this org read this transcription?" and nothing else.
// It applies exactly the refusals GetByID applies — unknown id, org mismatch,
// deleted row, and returns nil otherwise.
//
// It deliberately performs no content decryption and no source-metadata load:
// callers that only need the authorization answer (the generate-all endpoint
// probing access before a state change) should not pay for an
// AES-GCM open of a transcript up to 500KB plus a JSON parse and a media
// lookup whose results they immediately discard.
func (s *TranscriptionService) ProbeAccess(ctx context.Context, orgID, transcriptionID string) error {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return err
	}
	if trans.OrganizationID != orgID {
		return domain.ErrNotFound
	}
	if trans.Status == domain.TranscriptionStatusDeleted {
		return domain.ErrNotFound
	}
	return nil
}

// decryptCompletedContent decrypts and hydrates the transcript content when
// the transcription is completed and carries encrypted data.
func (s *TranscriptionService) decryptCompletedContent(ctx context.Context, orgID string, trans *domain.Transcription) error {
	if trans.Status != domain.TranscriptionStatusCompleted || len(trans.ContentEncrypted) == 0 {
		return nil
	}
	sealed := &crypto.SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    trans.ContentEncrypted,
		Nonce:         trans.ContentNonce,
		WrappedDEK:    trans.WrappedDEK,
		WrappingNonce: trans.WrappingNonce,
	}

	plaintext, openErr := s.envelope.OpenField(ctx, orgID, sealed)
	if openErr != nil {
		return fmt.Errorf("decrypt transcription content: %w", openErr)
	}

	var content domain.TranscriptionContent
	if unmarshalErr := json.Unmarshal(plaintext, &content); unmarshalErr != nil {
		return fmt.Errorf("unmarshal transcription content: %w", unmarshalErr)
	}

	trans.FullTranscript = content.FullTranscript
	trans.Utterances = content.Utterances
	trans.SpeakerMap = content.SpeakerMap
	trans.SuggestedSpeakerMap = content.SuggestedSpeakerMap
	trans.SuggestionsGenerated = content.SuggestionsGenerated
	return nil
}

// loadSourceMetadata populates media filename, title, and description.
func (s *TranscriptionService) loadSourceMetadata(ctx context.Context, trans *domain.Transcription) error {
	media, err := s.mediaRepo.GetByID(ctx, trans.MediaID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("load media: %w", err)
	}
	if media == nil {
		return nil
	}
	trans.MediaFilename = media.Filename
	trans.MediaTitle = media.Title
	trans.MediaDescription = media.Description
	trans.MediaStatus = media.Status
	return nil
}

// List returns the current organization's standard transcription page and total.
func (s *TranscriptionService) List(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Transcription, int64, error) {
	items, err := s.transRepo.ListByOrganization(ctx, orgID, limit, offset, search)
	if err != nil {
		return nil, 0, fmt.Errorf("list transcriptions: %w", err)
	}

	total, err := s.transRepo.CountByOrganization(ctx, orgID, search)
	if err != nil {
		return nil, 0, fmt.Errorf("count transcriptions: %w", err)
	}

	return items, total, nil
}

// ListFiltered retrieves transcriptions with optional filters.
// Delegates to the repository's ListByOrganizationFiltered.
func (s *TranscriptionService) ListFiltered(ctx context.Context, orgID string, filter port.TranscriptionListFilter) ([]*domain.Transcription, error) {
	items, err := s.transRepo.ListByOrganizationFiltered(ctx, orgID, filter)
	if err != nil {
		return nil, fmt.Errorf("list transcriptions filtered: %w", err)
	}
	return items, nil
}

// ListCompletedForTranscriptSearch returns a bounded keyset page of encrypted
// transcript candidates. Callers must use GetByID before reading content.
func (s *TranscriptionService) ListCompletedForTranscriptSearch(
	ctx context.Context,
	orgID string,
	cursor *port.TranscriptSearchCursor,
	limit int,
) ([]*domain.Transcription, error) {
	items, err := s.transRepo.ListCompletedForTranscriptSearch(ctx, orgID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list transcript search candidates: %w", err)
	}
	return items, nil
}

// GetByMediaID returns the latest transcription for the media, decrypted if completed.
func (s *TranscriptionService) GetByMediaID(ctx context.Context, orgID, mediaID string) (*domain.Transcription, error) {
	if orgID == "" || mediaID == "" {
		return nil, fmt.Errorf("orgID and mediaID required: %w", domain.ErrInvalidInput)
	}
	trans, err := s.transRepo.GetByMediaID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if trans.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// Decrypt content if completed.
	if trans.Status == domain.TranscriptionStatusCompleted && len(trans.ContentEncrypted) > 0 {
		sealed := &crypto.SealedFieldMeta{
			Algorithm:     "AES-256-GCM",
			Ciphertext:    trans.ContentEncrypted,
			Nonce:         trans.ContentNonce,
			WrappedDEK:    trans.WrappedDEK,
			WrappingNonce: trans.WrappingNonce,
		}
		plaintext, openErr := s.envelope.OpenField(ctx, orgID, sealed)
		if openErr != nil {
			return nil, fmt.Errorf("decrypt transcription content: %w", openErr)
		}
		var content domain.TranscriptionContent
		if err := json.Unmarshal(plaintext, &content); err != nil {
			return nil, fmt.Errorf("unmarshal transcription content: %w", err)
		}
		trans.FullTranscript = content.FullTranscript
		trans.Utterances = content.Utterances
		trans.SpeakerMap = content.SpeakerMap
		trans.SuggestedSpeakerMap = content.SuggestedSpeakerMap
		trans.SuggestionsGenerated = content.SuggestionsGenerated
	}
	return trans, nil
}

// UpdateSpeakers applies a human-confirmed speaker-name map to a completed transcription.
func (s *TranscriptionService) UpdateSpeakers(ctx context.Context, orgID, transcriptionID string, speakerMap map[string]string) error {
	return s.updateSpeakersInternal(ctx, orgID, transcriptionID, speakerMap)
}

// UpdateSpeakerLabels applies a partial human-confirmed speaker-name patch to a
// completed transcription. The patch is merged inside the encrypted-content CAS
// loop so concurrent speaker edits are preserved across retries.
func (s *TranscriptionService) UpdateSpeakerLabels(ctx context.Context, orgID, transcriptionID string, speakerLabels map[string]string) error {
	if err := s.checkSpeakerMutationAllowed(ctx, orgID, transcriptionID); err != nil {
		return err
	}
	cleaned, err := validateSpeakerLabelPatch(speakerLabels)
	if err != nil {
		return err
	}
	if _, err := s.mutateSpeakerContent(ctx, orgID, transcriptionID, func(trans *domain.Transcription, content *domain.TranscriptionContent) error {
		indexes, err := speakerIndexesForContent(content, trans.SpeakerCount)
		if err != nil {
			return err
		}
		indexSet := speakerIndexSet(indexes)
		if len(indexSet) == 0 {
			return fmt.Errorf("transcription has no diarized speakers: %w", domain.ErrInvalidInput)
		}
		if content.SpeakerMap == nil {
			content.SpeakerMap = make(map[string]string, len(cleaned))
		}
		fillSpeakerLabelDefaults(content, indexes)
		for index, name := range cleaned {
			if !indexSet[index] {
				return fmt.Errorf("unknown speaker index %d: %w", index, domain.ErrInvalidInput)
			}
			key := strconv.Itoa(index)
			content.SpeakerMap[key] = name
			if name != "" {
				delete(content.SuggestedSpeakerMap, key)
			}
		}
		return domain.ValidateUniqueSpeakerLabels(content.SpeakerMap)
	}); err != nil {
		return err
	}

	s.logger.Info("speaker labels updated",
		"transcription_id", transcriptionID,
		"org_id", orgID,
		"speaker_label_count", len(speakerLabels),
	)

	return nil
}

func validateSpeakerLabelPatch(speakerLabels map[string]string) (map[int]string, error) {
	cleaned := make(map[int]string, len(speakerLabels))
	for rawIndex, label := range speakerLabels {
		index, ok := domain.SpeakerIndexFromIdentifier(rawIndex)
		if !ok {
			return nil, fmt.Errorf("speaker label index %q is invalid: %w", rawIndex, domain.ErrInvalidInput)
		}
		if _, exists := cleaned[index]; exists {
			return nil, fmt.Errorf("duplicate speaker label index %d: %w", index, domain.ErrInvalidInput)
		}
		if err := domain.ValidateSpeakerLabel(index, label); err != nil {
			return nil, err
		}
		cleaned[index] = strings.TrimSpace(label)
	}
	if err := validateUniqueCanonicalSpeakerLabels(cleaned); err != nil {
		return nil, err
	}
	return cleaned, nil
}

func validateUniqueCanonicalSpeakerLabels(speakerLabels map[int]string) error {
	asStringMap := make(map[string]string, len(speakerLabels))
	for index, label := range speakerLabels {
		asStringMap[strconv.Itoa(index)] = label
	}
	return domain.ValidateUniqueSpeakerLabels(asStringMap)
}

func speakerIndexesForContent(content *domain.TranscriptionContent, speakerCount int) ([]int, error) {
	indexes, err := domain.SpeakerIndexesFromUtterances(content.Utterances)
	if err != nil {
		return nil, err
	}
	if len(indexes) == 0 && speakerCount > 0 {
		indexes = make([]int, 0, speakerCount)
		for i := 0; i < speakerCount; i++ {
			indexes = append(indexes, i)
		}
	}
	return indexes, nil
}

func speakerIndexSet(indexes []int) map[int]bool {
	indexSet := make(map[int]bool, len(indexes))
	for _, index := range indexes {
		indexSet[index] = true
	}
	return indexSet
}

func fillSpeakerLabelDefaults(content *domain.TranscriptionContent, indexes []int) {
	for _, index := range indexes {
		key := fmt.Sprintf("%d", index)
		if content.SpeakerMap[key] == "" {
			content.SpeakerMap[key] = domain.DefaultSpeakerLabel(index)
		}
	}
}

func (s *TranscriptionService) mutateSpeakerContent(
	ctx context.Context,
	orgID, transcriptionID string,
	mutate func(*domain.Transcription, *domain.TranscriptionContent) error,
) (*domain.Transcription, error) {
	return s.mutateContentWithReloadGuard(ctx, orgID, transcriptionID, nil, mutate)
}

// maxContentMutateAttempts bounds the CAS retry loop (Power-of-10 rule 2).
const maxContentMutateAttempts = 3

// decryptContent decrypts a transcription's content envelope into a
// TranscriptionContent. The caller must have verified org ownership.
func (s *TranscriptionService) decryptContent(ctx context.Context, orgID string, trans *domain.Transcription) (domain.TranscriptionContent, error) {
	var content domain.TranscriptionContent
	sealed := &crypto.SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    trans.ContentEncrypted,
		Nonce:         trans.ContentNonce,
		WrappedDEK:    trans.WrappedDEK,
		WrappingNonce: trans.WrappingNonce,
	}
	plaintext, err := s.envelope.OpenField(ctx, orgID, sealed)
	if err != nil {
		return content, fmt.Errorf("decrypt transcription content: %w", err)
	}
	if err := json.Unmarshal(plaintext, &content); err != nil {
		return content, fmt.Errorf("unmarshal transcription content: %w", err)
	}
	return content, nil
}

// mutateContent loads the transcription, decrypts its content envelope, applies
// mutate, then re-encrypts and persists via an optimistic compare-and-swap
// guarded by the content nonce read at load time. On a CAS miss (a concurrent
// writer committed first) it retries with a fresh reload, bounded to
// maxContentMutateAttempts. Returns domain.ErrConflict if all attempts lose.
// Authorization is the caller's job; the org-ownership check here is defense-in-depth on each reload. Status and content presence are re-validated because they can change during a concurrent update.
func (s *TranscriptionService) mutateContent(
	ctx context.Context, orgID, transcriptionID string,
	mutate func(*domain.TranscriptionContent) error,
) (*domain.Transcription, error) {
	return s.mutateContentWithReloadGuard(ctx, orgID, transcriptionID, nil, func(_ *domain.Transcription, content *domain.TranscriptionContent) error {
		return mutate(content)
	})
}

func (s *TranscriptionService) mutateContentWithReloadGuard(
	ctx context.Context,
	orgID, transcriptionID string,
	reloadGuard func(*domain.Transcription) error,
	mutate func(*domain.Transcription, *domain.TranscriptionContent) error,
) (*domain.Transcription, error) {
	for attempt := 0; attempt < maxContentMutateAttempts; attempt++ {
		trans, content, expectedNonce, err := s.loadContentForMutation(ctx, orgID, transcriptionID, reloadGuard)
		if err != nil {
			return nil, err
		}

		if mutateErr := mutate(trans, &content); mutateErr != nil {
			return nil, mutateErr
		}

		committed, err := s.resealAndCommit(ctx, orgID, trans, &content, expectedNonce)
		if err != nil {
			return nil, err
		}
		if committed {
			return trans, nil
		}
		// CAS miss: another writer committed; reload and retry.
	}
	return nil, domain.ErrConflict
}

func (s *TranscriptionService) loadContentForMutation(
	ctx context.Context,
	orgID, transcriptionID string,
	reloadGuard func(*domain.Transcription) error,
) (*domain.Transcription, domain.TranscriptionContent, []byte, error) {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return nil, domain.TranscriptionContent{}, nil, err
	}
	if trans.OrganizationID != orgID {
		return nil, domain.TranscriptionContent{}, nil, domain.ErrNotFound
	}
	if validateErr := validateMutableContentTranscription(trans); validateErr != nil {
		return nil, domain.TranscriptionContent{}, nil, validateErr
	}
	if reloadGuard != nil {
		if guardErr := reloadGuard(trans); guardErr != nil {
			return nil, domain.TranscriptionContent{}, nil, guardErr
		}
	}

	content, err := s.decryptContent(ctx, orgID, trans)
	if err != nil {
		return nil, domain.TranscriptionContent{}, nil, err
	}
	return trans, content, trans.ContentNonce, nil
}

func validateMutableContentTranscription(trans *domain.Transcription) error {
	if trans.Status != domain.TranscriptionStatusCompleted {
		return fmt.Errorf("transcription is not completed: %w", domain.ErrInvalidInput)
	}
	if len(trans.ContentEncrypted) == 0 {
		return fmt.Errorf("transcription has no content: %w", domain.ErrInvalidInput)
	}
	return nil
}

// resealAndCommit re-encrypts content, swaps the sealed fields onto trans, and
// runs the optimistic CAS guarded by expectedNonce. On a successful swap it
// copies the mutated speaker fields back onto trans and reports committed=true.
// A CAS miss returns (false, nil) so the caller can reload and retry.
func (s *TranscriptionService) resealAndCommit(
	ctx context.Context, orgID string, trans *domain.Transcription,
	content *domain.TranscriptionContent, expectedNonce []byte,
) (bool, error) {
	updated, marshalErr := json.Marshal(content)
	if marshalErr != nil {
		return false, fmt.Errorf("marshal updated content: %w", marshalErr)
	}
	newSealed, sealErr := s.envelope.SealField(ctx, orgID, updated)
	if sealErr != nil {
		return false, fmt.Errorf("re-encrypt transcription content: %w", sealErr)
	}
	trans.ContentEncrypted = newSealed.Ciphertext
	trans.ContentNonce = newSealed.Nonce
	trans.WrappedDEK = newSealed.WrappedDEK
	trans.WrappingNonce = newSealed.WrappingNonce

	ok, casErr := s.transRepo.UpdateContentCAS(ctx, trans, expectedNonce)
	if casErr != nil {
		return false, fmt.Errorf("update transcription content: %w", casErr)
	}
	if !ok {
		return false, nil
	}
	trans.SpeakerMap = content.SpeakerMap
	trans.SuggestedSpeakerMap = content.SuggestedSpeakerMap
	trans.SuggestionsGenerated = content.SuggestionsGenerated
	return true, nil
}

func (s *TranscriptionService) updateSpeakersInternal(ctx context.Context, orgID, transcriptionID string, speakerMap map[string]string) error {
	if err := s.checkSpeakerMutationAllowed(ctx, orgID, transcriptionID); err != nil {
		return err
	}

	// Persist via the race-safe CAS path. Authorization/validation above is
	// done once, up front; only the read→decrypt→modify→encrypt→write block is
	// routed through mutateContent so concurrent writers cannot clobber a
	// just-confirmed name.
	if _, err := s.mutateSpeakerContent(ctx, orgID, transcriptionID, func(_ *domain.Transcription, content *domain.TranscriptionContent) error {
		content.SpeakerMap = speakerMap
		// Confirming a name clears its AI suggestion: an index must never live
		// in both maps. An empty incoming value is not a confirmation, so its
		// suggestion is preserved.
		for index, name := range speakerMap {
			if name != "" {
				delete(content.SuggestedSpeakerMap, index)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	s.logger.Info("speakers updated",
		"transcription_id", transcriptionID,
		"org_id", orgID,
		"speaker_count", len(speakerMap),
	)

	return nil
}

func (s *TranscriptionService) checkSpeakerMutationAllowed(ctx context.Context, orgID, transcriptionID string) error {
	// 1. Load transcription.
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return err
	}

	// 2. Org isolation.
	if trans.OrganizationID != orgID {
		return domain.ErrNotFound
	}

	// 3. Must be completed.
	if trans.Status != domain.TranscriptionStatusCompleted {
		return fmt.Errorf("transcription is not completed: %w", domain.ErrInvalidInput)
	}

	// 4. Must have encrypted content.
	if len(trans.ContentEncrypted) == 0 {
		return fmt.Errorf("transcription has no content: %w", domain.ErrInvalidInput)
	}

	return nil
}

// Delete soft-deletes a transcription after verifying org isolation.
func (s *TranscriptionService) Delete(ctx context.Context, orgID, transcriptionID string) error {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return err
	}

	if trans.OrganizationID != orgID || trans.Status == domain.TranscriptionStatusDeleted {
		return domain.ErrNotFound
	}

	// Atomically soft-delete transcription and all child summaries
	// in a single database statement — no partial cascade possible.
	if err := s.transRepo.CascadeDelete(ctx, transcriptionID); err != nil {
		return fmt.Errorf("cascade delete transcription %s: %w", transcriptionID, err)
	}

	s.logger.Info("transcription deleted",
		"transcription_id", transcriptionID,
		"org_id", orgID,
	)

	return nil
}

// GetRecentActivity returns the N most recent transcriptions
// with their summary statuses aggregated. Composes ListRecentActivity (repo) with
// ListSummaryStatusesByTranscriptionIDs (summary repo).
func (s *TranscriptionService) GetRecentActivity(ctx context.Context, orgID string, limit int) ([]domain.RecentActivityItem, error) {
	transcriptions, err := s.transRepo.ListRecentActivity(ctx, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent activity: %w", err)
	}

	if len(transcriptions) == 0 {
		return nil, nil
	}

	// Collect transcription IDs for batch summary lookup.
	ids := make([]string, len(transcriptions))
	for i, t := range transcriptions {
		ids[i] = t.ID
	}

	summaryStatuses, err := s.summaryRepo.ListSummaryStatusesByTranscriptionIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list summary statuses: %w", err)
	}

	// Combine into RecentActivityItems.
	items := make([]domain.RecentActivityItem, len(transcriptions))
	for i, t := range transcriptions {
		items[i] = domain.RecentActivityItem{
			Transcription:   t,
			SummaryStatuses: summaryStatuses[t.ID],
		}
	}

	return items, nil
}
