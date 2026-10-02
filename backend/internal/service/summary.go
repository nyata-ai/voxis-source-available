// Copyright © 2026 PT. Karya Nyata Teknologi (Nyata.AI).
//
// This file is part of Voxis Source-Available. Use, modification, and distribution are
// governed by the license accompanying this distribution.

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// summaryContent is the JSON structure encrypted and stored with a
// completed summary. It contains the generated summary text.
type summaryContent struct {
	Content string `json:"content"`
}

type summaryMediaInfo struct {
	filename    string
	audioName   string
	description string
}

// SummaryService orchestrates creating summaries, processing AI results,
// encrypting/decrypting content, and managing summary lifecycle.
type SummaryService struct {
	summaryRepo       port.SummaryRepository
	transRepo         port.TranscriptionRepository
	mediaRepo         port.MediaRepository
	envelope          *crypto.EnvelopeService
	logger            *slog.Logger
	highStakesEnabled bool

	// Optional: privilege orchestrator. When set, Create + Regenerate reject
	// privilege media (use the privilege workspace endpoint instead) and
	// SummarizeWorker calls OnSummaryCompleted after persistence.
	privilegeSvc *PrivilegeService
}

// SummaryServiceOption configures optional SummaryService behavior.
type SummaryServiceOption func(*SummaryService)

// WithHighStakesEnabled controls whether regular summary creation may snapshot
// high_stakes=true. Privilege summaries bypass SummaryService and keep their own
// policy.
func WithHighStakesEnabled(enabled bool) SummaryServiceOption {
	return func(s *SummaryService) {
		s.highStakesEnabled = enabled
	}
}

// HighStakesEnabled reports whether regular high-stakes creation is enabled.
func (s *SummaryService) HighStakesEnabled() bool {
	return s.highStakesEnabled
}

// NewSummaryService creates a new SummaryService.
func NewSummaryService(
	summaryRepo port.SummaryRepository,
	transRepo port.TranscriptionRepository,
	mediaRepo port.MediaRepository,
	envelope *crypto.EnvelopeService,
	logger *slog.Logger,
	opts ...SummaryServiceOption,
) *SummaryService {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &SummaryService{
		summaryRepo: summaryRepo,
		transRepo:   transRepo,
		mediaRepo:   mediaRepo,
		envelope:    envelope,
		logger:      logger,
		// Unit tests and non-server construction keep the capability available;
		// the server wires the deployment flag explicitly.
		highStakesEnabled: true,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// SetPrivilegeService wires the PrivilegeService for direct-access guards
// and the OnSummaryCompleted lifecycle hook.
func (s *SummaryService) SetPrivilegeService(p *PrivilegeService) {
	s.privilegeSvc = p
}

// Create creates a new pending summary for a completed transcription.
// Validates: transcription exists, belongs to org, is completed,
// no active summary of the same type exists (ErrConflict).
func (s *SummaryService) Create(ctx context.Context, orgID, transcriptionID, summaryType string, highStakes bool) (*domain.Summary, error) {
	return s.CreateWithProfile(ctx, orgID, transcriptionID, summaryType, highStakes, domain.SummaryProfileGeneralProfessional)
}

// CreateWithProfile creates a pending summary with an initiating-user profile
// snapshot. Invalid values fall back to General Professional.
func (s *SummaryService) CreateWithProfile(
	ctx context.Context,
	orgID, transcriptionID, summaryType string,
	highStakes bool,
	summaryProfile string,
) (*domain.Summary, error) {
	// 1. Validate summary type via domain constructor (catches invalid types).
	summary, err := domain.NewSummary(orgID, transcriptionID, summaryType)
	if err != nil {
		return nil, err
	}
	if highStakes && !s.highStakesEnabled {
		return nil, fmt.Errorf("high-stakes summaries are disabled: %w", domain.ErrInvalidInput)
	}
	summary.HighStakes = highStakes
	summary.SummaryProfile = domain.NormalizeSummaryProfile(summaryProfile)

	// 2. Load transcription.
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return nil, err
	}

	// 3. Org isolation.
	if trans.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// 4. Transcription must be completed.
	if trans.Status != domain.TranscriptionStatusCompleted {
		return nil, fmt.Errorf("transcription is not completed (status=%s): %w", trans.Status, domain.ErrInvalidInput)
	}

	// 4.5. Privilege guard (Finding #6): block regular summary creation on
	// privilege media. Use POST /privilege/:mediaId/summarize instead.
	if s.privilegeSvc != nil && trans.MediaID != "" {
		if pErr := s.privilegeSvc.IsRegularSummaryAllowed(ctx, orgID, trans.MediaID); pErr != nil {
			return nil, pErr
		}
	}

	// 5. Check for existing active summary of the same type.
	_, err = s.summaryRepo.GetActiveByTranscriptionAndType(ctx, transcriptionID, summaryType)
	if err == nil {
		// Found an active summary of this type.
		return nil, fmt.Errorf("an active summary of type %q already exists for this transcription: %w", summaryType, domain.ErrConflict)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("check existing summary: %w", err)
	}

	// 6. Persist.
	if err := s.summaryRepo.Create(ctx, summary); err != nil {
		return nil, fmt.Errorf("persist summary: %w", err)
	}

	s.logger.Info("summary created",
		"summary_id", summary.ID,
		"transcription_id", transcriptionID,
		"org_id", orgID,
		"summary_type", summaryType,
		"high_stakes", highStakes,
	)

	return summary, nil
}

// ProcessResult processes a completed summary result from the AI provider.
// Encrypts the content JSON using EnvelopeService.SealField.
// Idempotent: returns nil if summary is already completed.
func (s *SummaryService) ProcessResult(ctx context.Context, summaryID string, result *port.SummaryResult) error {
	degradationCodes, err := domain.NormalizeSummaryDegradationCodes(result.DegradationCodes)
	if err != nil {
		return fmt.Errorf("validate summary degradation codes: %w", err)
	}

	// 1. Load summary.
	summary, err := s.summaryRepo.GetByID(ctx, summaryID)
	if err != nil {
		return fmt.Errorf("load summary: %w", err)
	}

	// 2. Idempotent: if already completed, return nil.
	// Also reject if failed or deleted.
	switch summary.Status {
	case domain.SummaryStatusCompleted:
		s.logger.Info("summary already completed, skipping",
			"summary_id", summaryID,
		)
		return nil
	case domain.SummaryStatusFailed, domain.SummaryStatusDeleted:
		s.logger.Info("summary in terminal state, ignoring result",
			"summary_id", summaryID,
			"status", summary.Status,
		)
		return nil
	}

	// 3. Build content JSON.
	content := summaryContent{
		Content: result.Content,
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal summary content: %w", err)
	}

	// 4. Encrypt content via envelope service.
	sealed, err := s.envelope.SealField(ctx, summary.OrganizationID, contentJSON)
	if err != nil {
		return fmt.Errorf("encrypt summary content: %w", err)
	}

	// 5. Store encrypted content on entity.
	summary.ContentEncrypted = sealed.Ciphertext
	summary.ContentNonce = sealed.Nonce
	summary.WrappedDEK = sealed.WrappedDEK
	summary.WrappingNonce = sealed.WrappingNonce

	if result.ExtractionJSON != "" {
		extraction, sealErr := s.envelope.SealField(ctx, summary.OrganizationID, []byte(result.ExtractionJSON))
		if sealErr != nil {
			return fmt.Errorf("encrypt summary extraction: %w", sealErr)
		}
		summary.ExtractionEncrypted = extraction.Ciphertext
		summary.ExtractionNonce = extraction.Nonce
		summary.ExtractionWrappedDEK = extraction.WrappedDEK
		summary.ExtractionWrappingNonce = extraction.WrappingNonce
	}
	if len(result.StructuredContent) != 0 {
		structured, sealErr := s.envelope.SealField(ctx, summary.OrganizationID, result.StructuredContent)
		if sealErr != nil {
			return fmt.Errorf("encrypt structured summary content: %w", sealErr)
		}
		summary.StructuredContentCiphertext = structured.Ciphertext
		summary.StructuredContentNonce = structured.Nonce
		summary.StructuredContentWrappedDEK = structured.WrappedDEK
		summary.StructuredContentWrappingNonce = structured.WrappingNonce
	}
	summary.StructuredSchemaVersion = result.StructuredSchemaVersion
	summary.PromptVersion = result.PromptVersion
	summary.Model = result.Model
	modelMetadata, err := summaryModelMetadata(result)
	if err != nil {
		return err
	}
	summary.ModelMetadata = modelMetadata
	summary.EndpointLocation = result.EndpointLocation
	summary.SourceVersion = result.SourceVersion
	summary.SourceHash = result.SourceHash
	summary.DegradationCodes = degradationCodes

	// 6. Mark as completed.
	summary.SetCompleted(result.WordCount, result.PromptTokens, result.CompletionTokens, result.ThinkingTokens)

	// 7. Persist.
	if err := s.summaryRepo.Update(ctx, summary); err != nil {
		return fmt.Errorf("update summary: %w", err)
	}

	s.logger.Info("summary result processed",
		"summary_id", summaryID,
		"word_count", result.WordCount,
		"prompt_tokens", result.PromptTokens,
		"completion_tokens", result.CompletionTokens,
		"thinking_tokens", result.ThinkingTokens,
	)

	return nil
}

func summaryModelMetadata(result *port.SummaryResult) ([]byte, error) {
	metadata := struct {
		ModelRevision   string `json:"model_revision,omitempty"`
		RuntimeRevision string `json:"runtime_revision,omitempty"`
		Quantization    string `json:"quantization,omitempty"`
		UsageAvailable  bool   `json:"usage_available"`
	}{
		ModelRevision:   result.ModelRevision,
		RuntimeRevision: result.RuntimeRevision,
		Quantization:    result.Quantization,
		UsageAvailable:  result.UsageAvailable,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal summary model metadata: %w", err)
	}
	return encoded, nil
}

// GetByID retrieves a summary, enforcing org isolation.
// Decrypts content via EnvelopeService.OpenField if completed.
// Also hydrates TranscriptionMediaFilename from transcription -> media.
//
// Privilege-mode summaries are deliberately hidden from this path so the
// only way to read privilege summary content is the privilege workspace
// endpoint. MCP/list_summaries, MCP/get_summary, REST GET /summaries, and
// the export tool all funnel through here.
func (s *SummaryService) GetByID(ctx context.Context, orgID, summaryID string) (*domain.Summary, error) {
	summary, err := s.summaryRepo.GetByID(ctx, summaryID)
	if err != nil {
		return nil, err
	}

	if err := validateSummaryAccess(summary, orgID); err != nil {
		return nil, err
	}

	if privileged, err := s.summaryIsPrivilege(ctx, summary); err != nil {
		return nil, err
	} else if privileged {
		return nil, domain.ErrNotFound
	}

	if err := s.decryptSummaryContent(ctx, orgID, summary); err != nil {
		return nil, err
	}

	if err := s.hydrateSummaryMedia(ctx, summary); err != nil {
		return nil, err
	}

	return summary, nil
}

// summaryIsPrivilege reports whether the summary's parent transcription is
// attached to privilege-mode media. Returns false (and no error) when the
// transcription has no media (URL-source summaries) or when the lookup fails
// in a way that suggests the data is gone — we fail closed at the caller by
// treating an error as a denied read.
func (s *SummaryService) summaryIsPrivilege(ctx context.Context, summary *domain.Summary) (bool, error) {
	trans, err := s.transRepo.GetByID(ctx, summary.TranscriptionID)
	if err != nil {
		return false, err
	}
	if trans.MediaID == "" {
		return false, nil
	}
	media, err := s.mediaRepo.GetByID(ctx, trans.MediaID)
	if err != nil {
		// Missing media on a regular-source summary means cascade cleanup is
		// in progress; treat as not-found rather than leaking content.
		return false, err
	}
	return media.IsPrivilege(), nil
}

// filterNonPrivilegeSummaries drops summaries whose parent transcription is
// attached to privilege-mode media. Used by every list path (ListFiltered,
// List, ListByTranscription) to keep privilege content out of bulk endpoints.
// O(transcriptions + media) database calls; both are cached in the request
// scope by the underlying repo when possible.
func (s *SummaryService) filterNonPrivilegeSummaries(ctx context.Context, items []*domain.Summary) []*domain.Summary {
	if len(items) == 0 {
		return items
	}
	// Decisions per transcription id:
	//   true  → summary may be shown (URL-source, or media is non-privilege)
	//   false → summary must be dropped (privilege OR lookup failed)
	// Initial absence means "not yet evaluated". Using a tri-state map keeps
	// the visible-filter loop branch-free and avoids the prior bug where an
	// empty mediaID accidentally passed the filter.
	transVisible := make(map[string]bool, len(items))
	mediaPriv := make(map[string]bool)
	for _, sum := range items {
		if _, seen := transVisible[sum.TranscriptionID]; seen {
			continue
		}
		trans, err := s.transRepo.GetByID(ctx, sum.TranscriptionID)
		if err != nil {
			// Fail closed: a missing or unreadable transcription means we
			// cannot evaluate privilege state, so we must drop the summary.
			transVisible[sum.TranscriptionID] = false
			continue
		}
		if trans.MediaID == "" {
			// URL-source transcription — no privilege media to check.
			transVisible[sum.TranscriptionID] = true
			continue
		}
		priv, cached := mediaPriv[trans.MediaID]
		if !cached {
			media, mErr := s.mediaRepo.GetByID(ctx, trans.MediaID)
			if mErr != nil {
				// Fail closed: missing media row could be a half-deleted
				// privilege record; do not leak any summary associated with it.
				priv = true
			} else {
				priv = media.IsPrivilege()
			}
			mediaPriv[trans.MediaID] = priv
		}
		transVisible[sum.TranscriptionID] = !priv
	}

	visible := items[:0]
	for _, sum := range items {
		if transVisible[sum.TranscriptionID] {
			visible = append(visible, sum)
		}
	}
	return visible
}

func validateSummaryAccess(summary *domain.Summary, orgID string) error {
	if summary.OrganizationID != orgID {
		return domain.ErrNotFound
	}

	if summary.Status == domain.SummaryStatusDeleted {
		return domain.ErrNotFound
	}

	return nil
}

func (s *SummaryService) decryptSummaryContent(ctx context.Context, orgID string, summary *domain.Summary) error {
	if summary.Status != domain.SummaryStatusCompleted {
		return nil
	}

	if len(summary.ContentEncrypted) != 0 {
		sealed := &crypto.SealedFieldMeta{
			Algorithm:     "AES-256-GCM",
			Ciphertext:    summary.ContentEncrypted,
			Nonce:         summary.ContentNonce,
			WrappedDEK:    summary.WrappedDEK,
			WrappingNonce: summary.WrappingNonce,
		}
		plaintext, err := s.envelope.OpenField(ctx, orgID, sealed)
		if err != nil {
			return fmt.Errorf("decrypt summary content: %w", err)
		}
		var content summaryContent
		if err := json.Unmarshal(plaintext, &content); err != nil {
			return fmt.Errorf("unmarshal summary content: %w", err)
		}
		summary.Content = content.Content
	}

	return s.decryptStructuredSummaryContent(ctx, orgID, summary)
}

func (s *SummaryService) decryptStructuredSummaryContent(ctx context.Context, orgID string, summary *domain.Summary) error {
	if len(summary.StructuredContentCiphertext) == 0 {
		return nil
	}

	sealed := &crypto.SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    summary.StructuredContentCiphertext,
		Nonce:         summary.StructuredContentNonce,
		WrappedDEK:    summary.StructuredContentWrappedDEK,
		WrappingNonce: summary.StructuredContentWrappingNonce,
	}
	plaintext, err := s.envelope.OpenField(ctx, orgID, sealed)
	if err != nil {
		return fmt.Errorf("decrypt structured summary content: %w", err)
	}
	summary.StructuredContent = plaintext

	return nil
}

func (s *SummaryService) hydrateSummaryMedia(ctx context.Context, summary *domain.Summary) error {
	mediaInfo, found, err := s.loadSummaryMediaForTranscription(ctx, summary.TranscriptionID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	applySummaryMediaInfo(summary, mediaInfo)
	return nil
}

// ListByTranscription returns all non-deleted summaries for a transcription,
// verifying org isolation by loading the transcription first.
// Hydrates TranscriptionMediaFilename on each item. Privilege-mode media
// transcriptions return ErrNotFound — privilege summaries are accessible only
// via the privilege workspace.
func (s *SummaryService) ListByTranscription(ctx context.Context, orgID, transcriptionID string) ([]*domain.Summary, error) {
	// 1. Verify transcription exists and belongs to org.
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		return nil, err
	}
	if trans.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// 2. Reject privilege media early so we don't even build the result set.
	// Fail closed: if the media row cannot be loaded (cascade in progress,
	// transient error), treat the transcription as inaccessible. The SQL
	// filter would also catch this in production, but the service guard must
	// agree so the contract holds regardless of repository implementation.
	if trans.MediaID != "" {
		media, mediaErr := s.mediaRepo.GetByID(ctx, trans.MediaID)
		if mediaErr != nil {
			return nil, domain.ErrNotFound
		}
		if media.IsPrivilege() {
			return nil, domain.ErrNotFound
		}
	}

	// 3. List summaries.
	items, err := s.summaryRepo.ListByTranscription(ctx, transcriptionID)
	if err != nil {
		return nil, fmt.Errorf("list summaries by transcription: %w", err)
	}
	// 4. Hydrate media filenames.
	s.hydrateMediaFilenames(ctx, items)

	return items, nil
}

// List retrieves summaries with pagination for an organization.
// Hydrates TranscriptionMediaFilename on each item. Privilege-derived
// summaries are post-filtered so they never appear in regular listings.
func (s *SummaryService) List(ctx context.Context, orgID string, limit, offset int, search string) ([]*domain.Summary, int64, error) {
	items, err := s.summaryRepo.ListByOrganization(ctx, orgID, limit, offset, search)
	if err != nil {
		return nil, 0, fmt.Errorf("list summaries: %w", err)
	}

	total, err := s.summaryRepo.CountByOrganization(ctx, orgID, search)
	if err != nil {
		return nil, 0, fmt.Errorf("count summaries: %w", err)
	}

	items = s.filterNonPrivilegeSummaries(ctx, items)
	// Hydrate media filenames.
	s.hydrateMediaFilenames(ctx, items)

	return items, total, nil
}

// ListFiltered retrieves summaries (both source types) with optional filters.
// Delegates to the repository's ListByOrganizationFiltered. Privilege-derived
// summaries are post-filtered so they never appear in regular listings; the
// underlying SQL also filters them out for performance.
func (s *SummaryService) ListFiltered(ctx context.Context, orgID string, filter port.SummaryListFilter) ([]*domain.Summary, error) {
	items, err := s.summaryRepo.ListByOrganizationFiltered(ctx, orgID, filter)
	if err != nil {
		return nil, fmt.Errorf("list summaries filtered: %w", err)
	}

	items = s.filterNonPrivilegeSummaries(ctx, items)
	// Hydrate media filenames for items not already populated by the query.
	s.hydrateMediaFilenames(ctx, items)

	return items, nil
}

// hydrateMediaFilenames populates media metadata fields on each summary by
// looking up transcription -> media. Errors are silently ignored (best-effort).
func (s *SummaryService) hydrateMediaFilenames(ctx context.Context, items []*domain.Summary) {
	// Cache to avoid repeated lookups for the same transcription.
	transMediaCache := make(map[string]summaryMediaInfo, len(items))

	for _, item := range items {
		if item.TranscriptionMediaFilename != "" &&
			item.TranscriptionAudioName != "" &&
			item.TranscriptionMediaDescription != "" {
			continue
		}

		cachedMedia, cached := transMediaCache[item.TranscriptionID]
		if !cached {
			loadedMedia, found, err := s.loadSummaryMediaForTranscription(ctx, item.TranscriptionID)
			if err != nil || !found {
				continue
			}
			transMediaCache[item.TranscriptionID] = loadedMedia
			cachedMedia = loadedMedia
		}

		applySummaryMediaInfo(item, cachedMedia)
	}
}

func (s *SummaryService) loadSummaryMediaForTranscription(
	ctx context.Context,
	transcriptionID string,
) (summaryMediaInfo, bool, error) {
	trans, err := s.transRepo.GetByID(ctx, transcriptionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return summaryMediaInfo{}, false, nil
		}
		return summaryMediaInfo{}, false, fmt.Errorf("load transcription: %w", err)
	}

	// URL transcriptions have no media — return empty info.
	if trans.MediaID == "" {
		return summaryMediaInfo{}, false, nil
	}

	media, err := s.mediaRepo.GetByID(ctx, trans.MediaID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return summaryMediaInfo{}, false, nil
		}
		return summaryMediaInfo{}, false, fmt.Errorf("load media: %w", err)
	}

	audioName := media.Filename
	if media.Title != "" {
		audioName = media.Title
	}

	return summaryMediaInfo{
		filename:    media.Filename,
		audioName:   audioName,
		description: media.Description,
	}, true, nil
}

func applySummaryMediaInfo(summary *domain.Summary, mediaInfo summaryMediaInfo) {
	if summary.TranscriptionMediaFilename == "" {
		summary.TranscriptionMediaFilename = mediaInfo.filename
	}
	if summary.TranscriptionAudioName == "" {
		summary.TranscriptionAudioName = mediaInfo.audioName
	}
	if summary.TranscriptionMediaDescription == "" {
		summary.TranscriptionMediaDescription = mediaInfo.description
	}
}

// Delete soft-deletes a summary after verifying org isolation.
func (s *SummaryService) Delete(ctx context.Context, orgID, summaryID string) error {
	summary, err := s.summaryRepo.GetByID(ctx, summaryID)
	if err != nil {
		return err
	}

	if summary.OrganizationID != orgID || summary.Status == domain.SummaryStatusDeleted {
		return domain.ErrNotFound
	}
	if s.privilegeSvc != nil {
		trans, terr := s.transRepo.GetByID(ctx, summary.TranscriptionID)
		if terr == nil && trans != nil && trans.MediaID != "" {
			if pErr := s.privilegeSvc.IsRegularSummaryAllowed(ctx, orgID, trans.MediaID); pErr != nil {
				return pErr
			}
		}
		if terr != nil && !errors.Is(terr, domain.ErrNotFound) {
			return terr
		}
	}

	if err := s.summaryRepo.Delete(ctx, summaryID); err != nil {
		return fmt.Errorf("delete summary: %w", err)
	}

	s.logger.Info("summary deleted",
		"summary_id", summaryID,
		"org_id", orgID,
	)

	return nil
}

// RegenerateResult holds information about both the old and new summary after
// regeneration, enabling the caller to undo the delete if job enqueue fails.
type RegenerateResult struct {
	NewSummary   *domain.Summary
	OldSummaryID string
	OldStatus    string
}

// Regenerate deletes the existing summary and creates a new pending one of the
// same type. Returns a RegenerateResult so the caller can undo the delete if
// the downstream job enqueue fails (atomicity safeguard).
func (s *SummaryService) Regenerate(ctx context.Context, orgID, summaryID string, highStakesOverride *bool) (*RegenerateResult, error) {
	return s.RegenerateWithProfile(ctx, orgID, summaryID, highStakesOverride, domain.SummaryProfileGeneralProfessional)
}

// RegenerateWithProfile replaces a summary with a new row that snapshots the
// profile effective for the user initiating regeneration.
func (s *SummaryService) RegenerateWithProfile(
	ctx context.Context,
	orgID, summaryID string,
	highStakesOverride *bool,
	summaryProfile string,
) (*RegenerateResult, error) {
	// 1. Load existing summary.
	existing, err := s.summaryRepo.GetByID(ctx, summaryID)
	if err != nil {
		return nil, err
	}

	// 2. Org isolation.
	if existing.OrganizationID != orgID {
		return nil, domain.ErrNotFound
	}

	// 3. If already deleted, nothing to regenerate.
	if existing.Status == domain.SummaryStatusDeleted {
		return nil, domain.ErrNotFound
	}

	// 4. Resolve type and transcription from the existing summary.
	summaryType := existing.SummaryType
	transcriptionID := existing.TranscriptionID
	oldStatus := existing.Status
	highStakes := existing.HighStakes
	if highStakesOverride != nil {
		highStakes = *highStakesOverride
	}

	// 4.5. Privilege guard BEFORE delete (self-review iter 1, Finding #12):
	// Otherwise the privilege summary would be deleted and Create would fail,
	// leaving the privilege media stuck below the 4-of-4 threshold.
	if s.privilegeSvc != nil {
		trans, terr := s.transRepo.GetByID(ctx, transcriptionID)
		if terr == nil && trans != nil && trans.MediaID != "" {
			if pErr := s.privilegeSvc.IsRegularSummaryAllowed(ctx, orgID, trans.MediaID); pErr != nil {
				return nil, pErr
			}
		}
	}

	// 5. Delete old summary.
	if delErr := s.summaryRepo.Delete(ctx, summaryID); delErr != nil {
		return nil, fmt.Errorf("delete old summary: %w", delErr)
	}

	// 6. Create new pending summary (reuse Create for validation).
	newSummary, err := s.CreateWithProfile(ctx, orgID, transcriptionID, summaryType, highStakes, summaryProfile)
	if err != nil {
		return nil, fmt.Errorf("create regenerated summary: %w", err)
	}

	s.logger.Info("summary regenerated",
		"old_summary_id", summaryID,
		"new_summary_id", newSummary.ID,
		"transcription_id", transcriptionID,
		"summary_type", summaryType,
		"org_id", orgID,
	)

	return &RegenerateResult{
		NewSummary:   newSummary,
		OldSummaryID: summaryID,
		OldStatus:    oldStatus,
	}, nil
}

// UndoDelete restores a soft-deleted summary to its previous status.
// Used to roll back a regeneration if the job enqueue fails.
func (s *SummaryService) UndoDelete(ctx context.Context, orgID, summaryID, restoreStatus string) error {
	return s.summaryRepo.UndoDelete(ctx, summaryID, orgID, restoreStatus)
}

// HandleFailure marks a summary as failed with an error message.
// Idempotent: returns nil if already in a terminal state (completed/failed/deleted).
func (s *SummaryService) HandleFailure(ctx context.Context, summaryID, errMsg string) error {
	summary, err := s.summaryRepo.GetByID(ctx, summaryID)
	if err != nil {
		return fmt.Errorf("load summary: %w", err)
	}

	// Guard: don't overwrite terminal states with failure.
	switch summary.Status {
	case domain.SummaryStatusCompleted:
		s.logger.Info("summary already completed, ignoring failure callback",
			"summary_id", summaryID,
		)
		return nil
	case domain.SummaryStatusFailed:
		s.logger.Info("summary already failed, skipping",
			"summary_id", summaryID,
		)
		return nil
	case domain.SummaryStatusDeleted:
		s.logger.Info("summary deleted, ignoring failure callback",
			"summary_id", summaryID,
		)
		return nil
	}

	summary.SetFailed(errMsg)

	if err := s.summaryRepo.Update(ctx, summary); err != nil {
		return fmt.Errorf("update summary: %w", err)
	}

	s.logger.Warn("summary failed",
		"summary_id", summaryID,
		"error_message", errMsg,
	)

	return nil
}
