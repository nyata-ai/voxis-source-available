package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/voxis/backend/internal/crypto"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

const (
	maxTranscriptLen   = 500_000
	defaultMaxAttempts = 3
)

// SummarizeJobArgs holds the arguments for a summarization job.
type SummarizeJobArgs struct {
	SummaryID       string `json:"summary_id"`
	TranscriptionID string `json:"transcription_id"`
	OrgID           string `json:"org_id"`
}

// Kind returns the unique job kind identifier.
func (SummarizeJobArgs) Kind() string { return "summarize" }

// InsertOpts provides default insertion options for all summarization jobs.
//
// Retryable is in the unique state list because a summarize job is a billable
// Gemini call. Between attempts a failed job sits in `retryable`, and the
// service re-enqueues for any summary still in `pending` — without this state a
// re-enqueue would insert a second job alongside the one waiting to retry, and
// both would eventually run and bill.
func (SummarizeJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "summarization",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStatePending,
				rivertype.JobStateAvailable,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// SummarizeWorker processes summarization jobs by decrypting transcription
// content, calling the AI provider, and persisting the result.
type SummarizeWorker struct {
	river.WorkerDefaults[SummarizeJobArgs]
	summaryRepo                     port.SummaryRepository
	transRepo                       port.TranscriptionRepository
	envelope                        *crypto.EnvelopeService
	provider                        port.SummaryProvider
	summarySvc                      *service.SummaryService
	privilegeSvc                    *service.PrivilegeService // optional; nil if privilege not wired
	logger                          *slog.Logger
	structuredOutput                bool
	structuredHighStakes            bool
	structuredLegacyFallbackEnabled bool
	structuredSourceChunkBytes      int
	summaryProfiles                 bool
	summaryJobTimeout               time.Duration
}

// SummarizeWorkerOption configures optional summary-generation behavior.
type SummarizeWorkerOption func(*SummarizeWorker)

// WithStructuredOutput enables the source-v1 and schema-validated pipeline.
// It is disabled by default so existing jobs keep their legacy request shape.
func WithStructuredOutput(enabled bool) SummarizeWorkerOption {
	return func(w *SummarizeWorker) { w.structuredOutput = enabled }
}

// WithStructuredHighStakes enables the anchored, two-pass path when both a
// high-stakes summary and structured output are requested. It is disabled by
// default so the existing structured-output precedence is unchanged.
func WithStructuredHighStakes(enabled bool) SummarizeWorkerOption {
	return func(w *SummarizeWorker) { w.structuredHighStakes = enabled }
}

// WithStructuredLegacyFallback controls the bounded legacy fallback after a
// structured summary failure. Providers that only support structured output
// must disable it.
func WithStructuredLegacyFallback(enabled bool) SummarizeWorkerOption {
	return func(w *SummarizeWorker) { w.structuredLegacyFallbackEnabled = enabled }
}

// WithStructuredSourceChunkBytes limits each structured provider source. The
// commercial default remains the existing source limit; the OSS composition
// derives this value from the local model's tested context budget.
func WithStructuredSourceChunkBytes(bytes int) SummarizeWorkerOption {
	return func(w *SummarizeWorker) { w.structuredSourceChunkBytes = bytes }
}

// WithSummaryProfilesEnabled toggles per-user summary profile resolution when
// building provider requests.
func WithSummaryProfilesEnabled(enabled bool) SummarizeWorkerOption {
	return func(w *SummarizeWorker) { w.summaryProfiles = enabled }
}

// WithSummaryJobTimeout sets an edition-specific ceiling for every summary job.
// An unset or non-positive value preserves the existing shared timeout policy.
func WithSummaryJobTimeout(timeout time.Duration) SummarizeWorkerOption {
	return func(w *SummarizeWorker) {
		if timeout > 0 {
			w.summaryJobTimeout = timeout
		}
	}
}

// SetPrivilegeService wires the PrivilegeService for OnSummaryCompleted hook.
func (w *SummarizeWorker) SetPrivilegeService(p *service.PrivilegeService) {
	w.privilegeSvc = p
}

// NewSummarizeWorker creates a new SummarizeWorker with the given dependencies.
func NewSummarizeWorker(
	summaryRepo port.SummaryRepository,
	transRepo port.TranscriptionRepository,
	envelope *crypto.EnvelopeService,
	provider port.SummaryProvider,
	summarySvc *service.SummaryService,
	logger *slog.Logger,
	opts ...SummarizeWorkerOption,
) *SummarizeWorker {
	if logger == nil {
		logger = slog.Default()
	}
	worker := &SummarizeWorker{
		summaryRepo:                     summaryRepo,
		transRepo:                       transRepo,
		envelope:                        envelope,
		provider:                        provider,
		summarySvc:                      summarySvc,
		logger:                          logger,
		structuredLegacyFallbackEnabled: true,
		structuredSourceChunkBytes:      service.MaxSummarySourceChunkBytes,
	}
	for _, opt := range opts {
		opt(worker)
	}
	return worker
}

// Timeout returns the maximum duration a summarization job may run.
func (w *SummarizeWorker) Timeout(job *river.Job[SummarizeJobArgs]) time.Duration {
	if w.summaryJobTimeout > 0 {
		return w.summaryJobTimeout
	}
	// River's Timeout hook has no request context. A best-effort row lookup lets
	// high-stakes jobs use the longer budget without changing job args.
	if w.summaryRepo != nil {
		summary, err := w.summaryRepo.GetByID(context.Background(), job.Args.SummaryID)
		if err == nil && (summary.HighStakes || w.structuredOutput) {
			return 20 * time.Minute
		}
	}
	return 10 * time.Minute
}

// Work executes the summarization pipeline:
//  1. Load and validate summary + transcription
//  2. Decrypt transcription content
//  3. Build and size-guard provider input
//  4. Call AI provider, classify errors
//  5. Persist result
func (w *SummarizeWorker) Work(ctx context.Context, job *river.Job[SummarizeJobArgs]) error {
	args := job.Args
	logger := w.logger.With(
		"summary_id", args.SummaryID,
		"transcription_id", args.TranscriptionID,
		"org_id", args.OrgID,
		"attempt", job.Attempt,
	)

	// 1. Load and validate summary + transcription.
	summary, trans, err := w.loadAndValidate(ctx, args, logger)
	if err != nil {
		recovered, hookErr := w.recoverPrivilegeHookForCompletedSummary(ctx, args, logger)
		if hookErr != nil {
			return hookErr
		}
		if recovered {
			return nil
		}
		return w.maybeFailOnLastAttempt(ctx, job, args.SummaryID, err, logger)
	}

	// 2. Decrypt and parse transcription content.
	content, err := w.decryptAndParse(ctx, args, trans, logger)
	if err != nil {
		return w.maybeFailOnLastAttempt(ctx, job, args.SummaryID, err, logger)
	}

	// 3. Preserve the exact legacy request path until structured output is
	// explicitly enabled. The structured path owns source-v1 and chunking.
	result, err := w.generateResult(ctx, args, summary, trans, content, logger)
	if err != nil {
		return w.maybeFailOnLastAttempt(ctx, job, args.SummaryID, err, logger)
	}

	// 5. Process result via SummaryService (encrypts + persists).
	if processErr := w.summarySvc.ProcessResult(ctx, args.SummaryID, result); processErr != nil {
		logger.Error("process result failed", "error", processErr)
		return w.maybeFailOnLastAttempt(ctx, job, args.SummaryID,
			fmt.Errorf("process summary result: %w", processErr), logger)
	}

	logger.Info("summary completed",
		"word_count", result.WordCount,
		"prompt_tokens", result.PromptTokens,
		"completion_tokens", result.CompletionTokens,
	)

	// Privilege lifecycle hook (Finding #2/#3): when 4/4 required summaries
	// are completed for a privilege transcription, this enqueues PrivilegeShredJob.
	if w.privilegeSvc != nil {
		if hookErr := w.privilegeSvc.OnSummaryCompleted(ctx, args.OrgID, args.TranscriptionID); hookErr != nil {
			logger.Error("privilege OnSummaryCompleted failed", "error", hookErr)
			return hookErr
		}
	}

	return nil
}

func (w *SummarizeWorker) recoverPrivilegeHookForCompletedSummary(ctx context.Context, args SummarizeJobArgs, logger *slog.Logger) (bool, error) {
	if w.privilegeSvc == nil {
		return false, nil
	}
	summary, err := w.summaryRepo.GetByID(ctx, args.SummaryID)
	if err != nil {
		// Best-effort probe: recovery only applies to an already-completed
		// summary. If the lookup fails, report "not recovered" so the caller
		// falls through to normal failure handling for the original error.
		return false, nil //nolint:nilerr // deliberate fail-soft; caller handles the original load error
	}
	if summary.Status != domain.SummaryStatusCompleted {
		return false, nil
	}
	if summary.OrganizationID != args.OrgID || summary.TranscriptionID != args.TranscriptionID {
		return false, nil
	}
	if hookErr := w.privilegeSvc.OnSummaryCompleted(ctx, args.OrgID, args.TranscriptionID); hookErr != nil {
		logger.Error("privilege OnSummaryCompleted recovery failed", "error", hookErr)
		return true, hookErr
	}
	return true, nil
}

// maybeFailOnLastAttempt checks whether an error is already a JobCancel
// (permanent failure already handled) or a transient error on the last retry
// attempt. For last-attempt transient errors, it marks the summary as failed
// and converts the error to a JobCancel to prevent River from silently
// discarding the job.
func (w *SummarizeWorker) maybeFailOnLastAttempt(
	ctx context.Context,
	job *river.Job[SummarizeJobArgs],
	summaryID string,
	err error,
	logger *slog.Logger,
) error {
	// If already a permanent failure (JobCancel), pass through unchanged.
	var cancelErr *river.JobCancelError
	if errors.As(err, &cancelErr) {
		return err
	}

	// On last attempt, mark the summary as failed so the user sees a clear status.
	if job.Attempt >= defaultMaxAttempts {
		logger.Warn("last attempt failed, marking summary as failed", "error", err)
		w.handleFailure(ctx, summaryID, fmt.Sprintf("failed after %d attempts: %v", job.Attempt, err), logger)
		return river.JobCancel(fmt.Errorf("exhausted %d attempts: %w", job.Attempt, err))
	}

	// Not the last attempt — return original error for River retry.
	return err
}

// loadAndValidate loads the summary and transcription records, validating
// preconditions and org isolation. Returns JobCancel for permanent failures.
func (w *SummarizeWorker) loadAndValidate(
	ctx context.Context,
	args SummarizeJobArgs,
	logger *slog.Logger,
) (*domain.Summary, *domain.Transcription, error) {
	// Load summary.
	summary, err := w.summaryRepo.GetByID(ctx, args.SummaryID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			logger.Warn("summary not found, canceling job")
			return nil, nil, river.JobCancel(fmt.Errorf("summary %s not found", args.SummaryID))
		}
		return nil, nil, fmt.Errorf("load summary: %w", err)
	}

	// Only process pending summaries.
	if summary.Status == domain.SummaryStatusCompleted {
		logger.Info("summary already completed, skipping")
		return nil, nil, river.JobCancel(fmt.Errorf("summary %s already completed", args.SummaryID))
	}
	if summary.Status != domain.SummaryStatusPending {
		logger.Info("summary not in pending state, skipping", "status", summary.Status)
		return nil, nil, river.JobCancel(fmt.Errorf("summary %s not pending (status=%s)", args.SummaryID, summary.Status))
	}

	// Org isolation for summary.
	if summary.OrganizationID != args.OrgID {
		logger.Error("org mismatch between job args and summary",
			"expected_org", args.OrgID,
			"actual_org", summary.OrganizationID,
		)
		return nil, nil, river.JobCancel(fmt.Errorf("org mismatch for summary %s", args.SummaryID))
	}
	if summary.TranscriptionID != args.TranscriptionID {
		logger.Error("transcription mismatch between job args and summary",
			"expected_transcription_id", summary.TranscriptionID,
			"actual_transcription_id", args.TranscriptionID,
		)
		w.handleFailure(ctx, args.SummaryID, "summary/transcription job mismatch", logger)
		return nil, nil, river.JobCancel(fmt.Errorf("transcription mismatch for summary %s", args.SummaryID))
	}

	// Load transcription.
	trans, err := w.transRepo.GetByID(ctx, args.TranscriptionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			logger.Warn("transcription not found, canceling job")
			return nil, nil, river.JobCancel(fmt.Errorf("transcription %s not found", args.TranscriptionID))
		}
		return nil, nil, fmt.Errorf("load transcription: %w", err)
	}

	// Validate transcription is completed.
	if trans.Status != domain.TranscriptionStatusCompleted {
		logger.Warn("transcription not completed, canceling job", "status", trans.Status)
		return nil, nil, river.JobCancel(fmt.Errorf("transcription %s not completed (status=%s)", args.TranscriptionID, trans.Status))
	}

	// Org isolation for transcription.
	if trans.OrganizationID != args.OrgID {
		logger.Error("org mismatch between job args and transcription",
			"expected_org", args.OrgID,
			"actual_org", trans.OrganizationID,
		)
		return nil, nil, river.JobCancel(fmt.Errorf("org mismatch for transcription %s", args.TranscriptionID))
	}

	return summary, trans, nil
}

// decryptAndParse decrypts the transcription content and unmarshals the JSON.
func (w *SummarizeWorker) decryptAndParse(
	ctx context.Context,
	args SummarizeJobArgs,
	trans *domain.Transcription,
	logger *slog.Logger,
) (*domain.TranscriptionContent, error) {
	sealed := &crypto.SealedFieldMeta{
		Algorithm:     "AES-256-GCM",
		Ciphertext:    trans.ContentEncrypted,
		Nonce:         trans.ContentNonce,
		WrappedDEK:    trans.WrappedDEK,
		WrappingNonce: trans.WrappingNonce,
	}
	plaintext, err := w.envelope.OpenField(ctx, args.OrgID, sealed)
	if err != nil {
		logger.Error("decryption failed", "error", err)
		return nil, fmt.Errorf("decrypt transcription content: %w", err)
	}

	var content domain.TranscriptionContent
	if unmarshalErr := json.Unmarshal(plaintext, &content); unmarshalErr != nil {
		logger.Error("unmarshal transcription content failed", "error", unmarshalErr)
		w.handleFailure(ctx, args.SummaryID, "invalid transcription content format", logger)
		return nil, river.JobCancel(fmt.Errorf("unmarshal transcription content: %w", unmarshalErr))
	}

	return &content, nil
}

func (w *SummarizeWorker) generateResult(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	trans *domain.Transcription,
	content *domain.TranscriptionContent,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	language := summaryLanguage(trans)
	if w.structuredOutput {
		if summary.HighStakes && w.structuredHighStakes {
			return w.generateStructuredHighStakesSummary(ctx, args, summary, content, language, logger)
		}
		if summary.HighStakes {
			// summary_id, transcription_id and org_id are already bound on logger.
			logger.Warn("high_stakes requested but structured-output path takes precedence",
				"summary_type", summary.SummaryType,
			)
		}
		return w.generateStructuredSummary(ctx, args, summary, content, language, logger)
	}
	requestText := summaryInputText(summary.SummaryType, summary.HighStakes, content)
	if len(requestText) > maxTranscriptLen {
		message := fmt.Sprintf("transcript too large (%d chars, max %d)", len(requestText), maxTranscriptLen)
		w.handleFailure(ctx, args.SummaryID, message, logger)
		return nil, river.JobCancel(errors.New(message))
	}
	return w.generateLegacySummary(ctx, args, summary, requestText, language, logger)
}

func (w *SummarizeWorker) generateStructuredSummary(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	content *domain.TranscriptionContent,
	language string,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	source, err := w.buildStructuredSource(content, language)
	if err != nil {
		return w.structuredLegacyFallback(ctx, args, summary, content, language, domain.SummaryDegradationStructuredSourceFallback, err, logger)
	}
	results := make([]*port.SummaryResult, 0, len(source.Chunks))
	for _, chunk := range source.Chunks {
		result, callErr := w.generateStructuredChunk(ctx, args, summary, source, chunk, language, logger)
		if callErr != nil {
			return w.recoverStructuredChunkFailure(ctx, args, summary, content, language, source, results, callErr, logger)
		}
		results = append(results, result)
	}
	merged, err := mergeStructuredSourceChunkResults(summary.SummaryType, source, results)
	if err != nil {
		return w.structuredFallbackWithTokens(ctx, args, summary, content, language, domain.SummaryDegradationStructuredValidationFallback, err, source, results, logger)
	}
	merged.DegradationCodes = append(merged.DegradationCodes, source.DegradationCodes...)
	return merged, nil
}

// generateStructuredHighStakesSummary keeps each source chunk intact for the
// evidence extraction and anchored structured generation passes. The merged
// summary is validated against the full deterministic source before it is
// persisted.
func (w *SummarizeWorker) generateStructuredHighStakesSummary(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	content *domain.TranscriptionContent,
	language string,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	source, err := w.buildStructuredSource(content, language)
	if err != nil {
		return w.structuredLegacyFallback(ctx, args, summary, content, language, domain.SummaryDegradationStructuredSourceFallback, err, logger)
	}
	anchored, ok := w.provider.(port.AnchoredSummaryProvider)
	if !ok {
		w.handleFailure(ctx, args.SummaryID, "high-stakes summary provider is not configured", logger)
		return nil, river.JobCancel(fmt.Errorf("high-stakes provider unavailable"))
	}
	results := make([]*port.SummaryResult, 0, len(source.Chunks))
	extractions := make([]*port.SummaryExtractionResult, 0, len(source.Chunks))
	for _, chunk := range source.Chunks {
		result, extraction, callErr := w.generateStructuredHighStakesChunk(ctx, args, summary, anchored, source, chunk, language, logger)
		if callErr != nil {
			return w.recoverStructuredChunkFailure(ctx, args, summary, content, language, source, results, callErr, logger)
		}
		results = append(results, result)
		extractions = append(extractions, extraction)
	}
	merged, err := mergeStructuredSourceChunkResults(summary.SummaryType, source, results)
	if err != nil {
		return w.structuredFallbackWithTokens(ctx, args, summary, content, language, domain.SummaryDegradationStructuredValidationFallback, err, source, results, logger)
	}
	extractionJSON, err := aggregateStructuredExtractions(source, extractions)
	if err != nil {
		return w.structuredFallbackWithTokens(ctx, args, summary, content, language, domain.SummaryDegradationStructuredValidationFallback, err, source, results, logger)
	}
	merged.ExtractionJSON = extractionJSON
	merged.DegradationCodes = append(merged.DegradationCodes, source.DegradationCodes...)
	return merged, nil
}

func (w *SummarizeWorker) generateStructuredHighStakesChunk(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	anchored port.AnchoredSummaryProvider,
	source *service.SummarySource,
	chunk service.SummarySourceChunk,
	language string,
	logger *slog.Logger,
) (*port.SummaryResult, *port.SummaryExtractionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	extraction, err := w.extractSummaryAnchorsWithRetry(ctx, anchored, port.SummaryExtractionRequest{
		Text:             chunk.Text,
		SummaryType:      summary.SummaryType,
		Language:         language,
		SummaryProfile:   w.requestProfile(summary),
		SourceVersion:    source.Version,
		SourceHash:       chunk.Hash,
		SourceChunkIndex: chunk.Index,
		SourceChunkCount: len(source.Chunks),
	}, logger)
	if err != nil {
		return nil, nil, w.handleProviderError(ctx, args.SummaryID, "extract structured summary anchors", err, logger)
	}
	if extraction == nil {
		return nil, nil, w.handleProviderError(ctx, args.SummaryID, "extract structured summary anchors", domain.ErrGenerationUnsupported, logger)
	}
	result, err := anchored.GenerateSummaryFromAnchors(ctx, port.AnchoredSummaryRequest{
		ExtractionJSON:   extraction.ExtractionJSON,
		Transcript:       chunk.Text,
		SummaryType:      summary.SummaryType,
		Language:         language,
		SummaryProfile:   w.requestProfile(summary),
		SourceVersion:    source.Version,
		SourceHash:       chunk.Hash,
		SourceChunkIndex: chunk.Index,
		SourceChunkCount: len(source.Chunks),
	})
	if err != nil {
		return nil, nil, w.handleProviderError(ctx, args.SummaryID, "generate structured summary from anchors", err, logger)
	}
	if result == nil {
		return nil, nil, w.handleProviderError(ctx, args.SummaryID, "generate structured summary from anchors", domain.ErrGenerationUnsupported, logger)
	}
	result.PromptTokens += extraction.PromptTokens
	result.CompletionTokens += extraction.CompletionTokens
	result.ThinkingTokens += extraction.ThinkingTokens
	result.UsageAvailable = result.UsageAvailable && extraction.UsageAvailable
	return result, extraction, nil
}

func (w *SummarizeWorker) buildStructuredSource(content *domain.TranscriptionContent, language string) (*service.SummarySource, error) {
	return service.BuildSummarySourceWithChunkLimit(content, language, w.structuredSourceChunkBytes)
}

func aggregateStructuredExtractions(source *service.SummarySource, extractions []*port.SummaryExtractionResult) (string, error) {
	if source == nil || len(source.Chunks) == 0 || len(source.Chunks) != len(extractions) {
		return "", structuredMergeError("extraction artifacts are incomplete")
	}
	type extractionChunk struct {
		Index      int             `json:"index"`
		SourceHash string          `json:"source_hash"`
		Artifact   json.RawMessage `json:"artifact"`
	}
	bundle := struct {
		Version    string            `json:"version"`
		SourceHash string            `json:"source_hash"`
		Chunks     []extractionChunk `json:"chunks"`
	}{
		Version:    source.Version,
		SourceHash: source.Hash,
		Chunks:     make([]extractionChunk, 0, len(extractions)),
	}
	for index, extraction := range extractions {
		if extraction == nil || !json.Valid([]byte(extraction.ExtractionJSON)) {
			return "", structuredMergeError("chunk %d has invalid extraction artifact", index+1)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(extraction.ExtractionJSON), &object); err != nil || object == nil {
			return "", structuredMergeError("chunk %d extraction artifact must be an object", index+1)
		}
		bundle.Chunks = append(bundle.Chunks, extractionChunk{
			Index:      source.Chunks[index].Index,
			SourceHash: source.Chunks[index].Hash,
			Artifact:   json.RawMessage(extraction.ExtractionJSON),
		})
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("marshal extraction artifacts: %w", err)
	}
	return string(encoded), nil
}

// recoverStructuredChunkFailure decides how a failed structured chunk call is
// handled: cancellation/deadline and JobCancel errors propagate unchanged,
// schema rejections and mid-stream failures degrade to the legacy path, and a
// first-chunk transient failure is returned for a normal River retry.
func (w *SummarizeWorker) recoverStructuredChunkFailure(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	content *domain.TranscriptionContent,
	language string,
	source *service.SummarySource,
	results []*port.SummaryResult,
	callErr error,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	var cancelErr *river.JobCancelError
	if errors.As(callErr, &cancelErr) || errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
		return nil, callErr
	}
	if errors.Is(callErr, domain.ErrStructuredRequestRejected) {
		return w.structuredFallbackWithTokens(ctx, args, summary, content, language, domain.SummaryDegradationStructuredValidationFallback, callErr, source, results, logger)
	}
	if len(results) > 0 {
		return w.structuredFallbackWithTokens(ctx, args, summary, content, language, domain.SummaryDegradationStructuredChunkFallback, callErr, source, results, logger)
	}
	return nil, callErr
}

// structuredFallbackWithTokens runs the legacy fallback and, on success, folds
// in tokens already consumed by prior structured chunks plus the source-level
// degradation codes.
func (w *SummarizeWorker) structuredFallbackWithTokens(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	content *domain.TranscriptionContent,
	language, code string,
	cause error,
	source *service.SummarySource,
	results []*port.SummaryResult,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	if !w.structuredLegacyFallbackEnabled {
		return nil, w.handleProviderError(ctx, args.SummaryID, "generate structured summary", cause, logger)
	}
	fallback, fallbackErr := w.structuredLegacyFallback(ctx, args, summary, content, language, code, cause, logger)
	if fallback != nil {
		addConsumedStructuredTokens(fallback, results)
		fallback.DegradationCodes = append(fallback.DegradationCodes, source.DegradationCodes...)
	}
	return fallback, fallbackErr
}

func addConsumedStructuredTokens(result *port.SummaryResult, prior []*port.SummaryResult) {
	for _, value := range prior {
		if value == nil {
			continue
		}
		result.PromptTokens += value.PromptTokens
		result.CompletionTokens += value.CompletionTokens
		result.ThinkingTokens += value.ThinkingTokens
	}
}

func (w *SummarizeWorker) generateStructuredChunk(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	source *service.SummarySource,
	chunk service.SummarySourceChunk,
	language string,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := w.provider.GenerateSummary(ctx, port.SummaryRequest{
		Text:             chunk.Text,
		SummaryType:      summary.SummaryType,
		Language:         language,
		SummaryProfile:   w.requestProfile(summary),
		SourceVersion:    source.Version,
		SourceHash:       chunk.Hash,
		SourceChunkIndex: chunk.Index,
		SourceChunkCount: len(source.Chunks),
	})
	if err != nil {
		return nil, w.handleProviderError(ctx, args.SummaryID, "generate structured summary", err, logger)
	}
	return result, nil
}

func (w *SummarizeWorker) generateLegacySummary(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	requestText, language string,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	if summary.HighStakes {
		return w.generateLegacyHighStakesSummary(ctx, args, summary, requestText, language, logger)
	}
	result, err := w.provider.GenerateSummary(ctx, port.SummaryRequest{
		Text:           requestText,
		SummaryType:    summary.SummaryType,
		Language:       language,
		SummaryProfile: w.requestProfile(summary),
	})
	if err != nil {
		return nil, w.handleProviderError(ctx, args.SummaryID, "generate summary", err, logger)
	}
	return result, nil
}

func (w *SummarizeWorker) generateLegacyHighStakesSummary(
	ctx context.Context,
	args SummarizeJobArgs,
	summary *domain.Summary,
	requestText, language string,
	logger *slog.Logger,
) (*port.SummaryResult, error) {
	anchored, ok := w.provider.(port.AnchoredSummaryProvider)
	if !ok {
		w.handleFailure(ctx, args.SummaryID, "high-stakes summary provider is not configured", logger)
		return nil, river.JobCancel(fmt.Errorf("high-stakes provider unavailable"))
	}

	extraction, err := w.extractSummaryAnchorsWithRetry(ctx, anchored, port.SummaryExtractionRequest{
		Text:           requestText,
		SummaryType:    summary.SummaryType,
		Language:       language,
		SummaryProfile: w.requestProfile(summary),
	}, logger)
	if err != nil {
		return nil, w.handleProviderError(ctx, args.SummaryID, "extract summary anchors", err, logger)
	}

	result, err := anchored.GenerateSummaryFromAnchors(ctx, port.AnchoredSummaryRequest{
		ExtractionJSON: extraction.ExtractionJSON,
		Transcript:     requestText,
		SummaryType:    summary.SummaryType,
		Language:       language,
		SummaryProfile: w.requestProfile(summary),
	})
	if err != nil {
		return nil, w.handleProviderError(ctx, args.SummaryID, "compress summary anchors", err, logger)
	}

	result.PromptTokens += extraction.PromptTokens
	result.CompletionTokens += extraction.CompletionTokens
	result.ThinkingTokens += extraction.ThinkingTokens
	result.ExtractionJSON = extraction.ExtractionJSON
	return result, nil
}

// extractSummaryAnchorsWithRetry runs the extraction pass, retrying once when
// the model stops at its output-token limit. On gemini-3.7-flash the
// extraction call sometimes degenerates into a repetition loop that runs to
// MAX_TOKENS; a fresh attempt usually completes normally, so one bounded
// in-attempt retry avoids permanently failing the high-stakes summary while
// capping the wasted token spend at two attempts.
func (w *SummarizeWorker) extractSummaryAnchorsWithRetry(
	ctx context.Context,
	anchored port.AnchoredSummaryProvider,
	req port.SummaryExtractionRequest,
	logger *slog.Logger,
) (*port.SummaryExtractionResult, error) {
	extraction, err := anchored.ExtractSummaryAnchors(ctx, req)
	if err == nil || !errors.Is(err, domain.ErrGenerationIncomplete) || ctx.Err() != nil {
		return extraction, err
	}
	logger.Warn("extraction pass stopped at output-token limit, retrying once",
		"summary_type", req.SummaryType,
		"error", err,
	)
	return anchored.ExtractSummaryAnchors(ctx, req)
}

func (w *SummarizeWorker) handleProviderError(
	ctx context.Context,
	summaryID, stage string,
	err error,
	logger *slog.Logger,
) error {
	if message, permanent := permanentGenerationFailureMessage(err); permanent {
		logger.Warn("provider returned permanent generation outcome", "stage", stage, "error", err)
		w.handleFailure(ctx, summaryID, message, logger)
		return river.JobCancel(fmt.Errorf("%s: %w", stage, err))
	}
	if errors.Is(err, domain.ErrStructuredRequestRejected) {
		logger.Warn("provider rejected structured request", "stage", stage, "error", err)
		return fmt.Errorf("%s: %w", stage, err)
	}
	logger.Error("provider error (transient)", "stage", stage, "error", err)
	return fmt.Errorf("%s: %w", stage, err)
}

func permanentGenerationFailureMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, domain.ErrContentBlocked):
		return "content blocked by safety filters", true
	case errors.Is(err, domain.ErrGenerationIncomplete):
		return "summary generation reached its output limit", true
	case errors.Is(err, domain.ErrGenerationUnsupported):
		return "summary generation returned an unsupported response", true
	default:
		return "", false
	}
}

func (w *SummarizeWorker) structuredLegacyFallback(
	ctx context.Context, args SummarizeJobArgs, summary *domain.Summary,
	content *domain.TranscriptionContent, language, code string, cause error, logger *slog.Logger,
) (*port.SummaryResult, error) {
	if !w.structuredLegacyFallbackEnabled {
		return nil, w.handleProviderError(ctx, args.SummaryID, "build structured summary source", cause, logger)
	}
	text := summaryInputText(summary.SummaryType, false, content)
	if len(text) > maxTranscriptLen {
		message := summarySourceFailureMessage(cause)
		w.handleFailure(ctx, args.SummaryID, message, logger)
		return nil, river.JobCancel(fmt.Errorf("structured fallback exceeds legacy capacity: %w", cause))
	}
	result, err := w.provider.GenerateSummary(ctx, port.SummaryRequest{
		Text: text, SummaryType: summary.SummaryType, Language: language,
		SummaryProfile: w.requestProfile(summary), ForceLegacy: true,
	})
	if err != nil {
		return nil, w.handleProviderError(ctx, args.SummaryID, "generate structured fallback", err, logger)
	}
	result.DegradationCodes = append(result.DegradationCodes, code)
	logger.Warn("structured summary degraded to legacy output", "code", code, "cause", cause)
	return result, nil
}

func (w *SummarizeWorker) requestProfile(summary *domain.Summary) string {
	if !w.summaryProfiles {
		return ""
	}
	return domain.NormalizeSummaryProfile(summary.SummaryProfile)
}

func mergeStructuredSourceChunkResults(
	summaryType string,
	source *service.SummarySource,
	results []*port.SummaryResult,
) (*port.SummaryResult, error) {
	if source == nil || len(source.Chunks) == 0 || len(source.Chunks) != len(results) {
		return nil, structuredMergeError("chunk results are incomplete")
	}
	parsed := make([]*domain.StructuredSummary, 0, len(results))
	for index, result := range results {
		if result == nil {
			return nil, structuredMergeError("chunk %d has no structured content", index+1)
		}
		chunkSource, err := structuredChunkSource(source, source.Chunks[index])
		if err != nil {
			return nil, structuredMergeError("chunk %d source: %v", index+1, err)
		}
		raw := result.StructuredContent
		if len(raw) == 0 {
			raw = []byte(result.Content)
		}
		response, err := service.ParseAndValidateStructuredSummary(summaryType, raw, chunkSource)
		if err != nil {
			return nil, structuredMergeError("chunk %d validation: %v", index+1, err)
		}
		result.DegradationCodes = append(result.DegradationCodes, response.DegradationCodes...)
		prefixStructuredItemIDs(response, index+1)
		parsed = append(parsed, response)
	}

	merged, err := mergeStructuredRoots(summaryType, parsed)
	if err != nil {
		return nil, structuredMergeError("merge: %v", err)
	}
	if err := validateStructuredResultProvenance(results); err != nil {
		return nil, err
	}
	service.NormalizeStructuredSummary(merged, source)
	if err := service.ValidateStructuredSummary(merged, source); err != nil {
		return nil, structuredMergeError("full-source validation: %v", err)
	}
	return renderMergedStructuredResult(merged, source, results)
}

func validateStructuredResultProvenance(results []*port.SummaryResult) error {
	if len(results) == 0 || results[0] == nil {
		return structuredMergeError("chunk results are incomplete")
	}
	first := results[0]
	for index, result := range results[1:] {
		if result == nil {
			return structuredMergeError("chunk %d has no structured content", index+2)
		}
		if result.PromptVersion != first.PromptVersion ||
			result.Model != first.Model ||
			result.EndpointLocation != first.EndpointLocation ||
			result.ModelRevision != first.ModelRevision ||
			result.RuntimeRevision != first.RuntimeRevision ||
			result.Quantization != first.Quantization {
			return structuredMergeError("chunk %d model or prompt provenance differs", index+2)
		}
	}
	return nil
}

func structuredChunkSource(source *service.SummarySource, chunk service.SummarySourceChunk) (*service.SummarySource, error) {
	wanted := make(map[string]struct{}, len(chunk.SegmentIDs))
	for _, id := range chunk.SegmentIDs {
		wanted[id] = struct{}{}
	}
	segments := make([]service.SummarySourceSegment, 0, len(wanted))
	for _, segment := range source.Segments {
		if _, ok := wanted[segment.ID]; ok {
			segments = append(segments, segment)
		}
	}
	if len(segments) != len(wanted) {
		return nil, errors.New("source segments do not match chunk")
	}
	return &service.SummarySource{
		Version:  source.Version,
		Language: source.Language,
		Segments: segments,
		Text:     chunk.Text,
		Hash:     chunk.Hash,
	}, nil
}

func mergeStructuredRoots(summaryType string, values []*domain.StructuredSummary) (*domain.StructuredSummary, error) {
	if len(values) == 0 {
		return nil, errors.New("no structured summaries")
	}
	switch summaryType {
	case domain.SummaryTypeGeneral:
		return mergeStructuredGeneral(values)
	case domain.SummaryTypeKeyPoints:
		return mergeStructuredKeyPoints(values)
	case domain.SummaryTypeActionItems:
		return mergeStructuredActionItems(values)
	case domain.SummaryTypeQAndA:
		return mergeStructuredQuestionAnswers(values)
	default:
		return nil, fmt.Errorf("unsupported summary type %q", summaryType)
	}
}

func mergeStructuredGeneral(values []*domain.StructuredSummary) (*domain.StructuredSummary, error) {
	first := values[0].General
	if first == nil {
		return nil, errors.New("missing general summary")
	}
	merged := &domain.StructuredGeneralSummary{SchemaVersion: first.SchemaVersion, MatrixLanguage: first.MatrixLanguage}
	for _, value := range values {
		if value.General == nil {
			return nil, errors.New("wrong structured summary type")
		}
		merged.Paragraphs = append(merged.Paragraphs, value.General.Paragraphs...)
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeGeneral, General: merged}, nil
}

func mergeStructuredKeyPoints(values []*domain.StructuredSummary) (*domain.StructuredSummary, error) {
	first := values[0].KeyPoints
	if first == nil {
		return nil, errors.New("missing key-points summary")
	}
	merged := &domain.StructuredKeyPointsSummary{SchemaVersion: first.SchemaVersion, MatrixLanguage: first.MatrixLanguage}
	for _, value := range values {
		if value.KeyPoints == nil {
			return nil, errors.New("wrong structured summary type")
		}
		merged.Items = append(merged.Items, value.KeyPoints.Items...)
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeKeyPoints, KeyPoints: merged}, nil
}

func mergeStructuredActionItems(values []*domain.StructuredSummary) (*domain.StructuredSummary, error) {
	first := values[0].ActionItems
	if first == nil {
		return nil, errors.New("missing action-items summary")
	}
	merged := &domain.StructuredActionItemsSummary{SchemaVersion: first.SchemaVersion, MatrixLanguage: first.MatrixLanguage}
	for _, value := range values {
		if value.ActionItems == nil {
			return nil, errors.New("wrong structured summary type")
		}
		merged.Items = append(merged.Items, value.ActionItems.Items...)
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeActionItems, ActionItems: merged}, nil
}

func mergeStructuredQuestionAnswers(values []*domain.StructuredSummary) (*domain.StructuredSummary, error) {
	first := values[0].QAndA
	if first == nil {
		return nil, errors.New("missing Q&A summary")
	}
	merged := &domain.StructuredQuestionAndAnswerSummary{SchemaVersion: first.SchemaVersion, MatrixLanguage: first.MatrixLanguage}
	for _, value := range values {
		if value.QAndA == nil {
			return nil, errors.New("wrong structured summary type")
		}
		merged.Items = append(merged.Items, value.QAndA.Items...)
	}
	return &domain.StructuredSummary{SummaryType: domain.SummaryTypeQAndA, QAndA: merged}, nil
}

func prefixStructuredItemIDs(summary *domain.StructuredSummary, chunkNumber int) {
	prefix := fmt.Sprintf("c%d-", chunkNumber)
	switch summary.SummaryType {
	case domain.SummaryTypeGeneral:
		for index := range summary.General.Paragraphs {
			summary.General.Paragraphs[index].ID = fmt.Sprintf("%sp%d", prefix, index+1)
		}
	case domain.SummaryTypeKeyPoints:
		for index := range summary.KeyPoints.Items {
			summary.KeyPoints.Items[index].ID = fmt.Sprintf("%si%d", prefix, index+1)
		}
	case domain.SummaryTypeActionItems:
		for index := range summary.ActionItems.Items {
			summary.ActionItems.Items[index].ID = fmt.Sprintf("%si%d", prefix, index+1)
		}
	case domain.SummaryTypeQAndA:
		for index := range summary.QAndA.Items {
			summary.QAndA.Items[index].ID = fmt.Sprintf("%si%d", prefix, index+1)
		}
	}
}

func renderMergedStructuredResult(
	merged *domain.StructuredSummary,
	source *service.SummarySource,
	results []*port.SummaryResult,
) (*port.SummaryResult, error) {
	content, err := service.RenderStructuredSummary(merged, source)
	if err != nil {
		return nil, structuredMergeError("render: %v", err)
	}
	encoded, err := service.MarshalStructuredSummary(merged)
	if err != nil {
		return nil, structuredMergeError("marshal: %v", err)
	}
	first := results[0]
	output := &port.SummaryResult{
		Content:                 content,
		StructuredContent:       encoded,
		StructuredSchemaVersion: domain.StructuredSummarySchemaVersion,
		PromptVersion:           first.PromptVersion,
		Model:                   first.Model,
		ModelRevision:           first.ModelRevision,
		RuntimeRevision:         first.RuntimeRevision,
		Quantization:            first.Quantization,
		EndpointLocation:        first.EndpointLocation,
		SourceVersion:           source.Version,
		SourceHash:              source.Hash,
		WordCount:               len(strings.Fields(content)),
		UsageAvailable:          first.UsageAvailable,
	}
	for _, result := range results {
		output.PromptTokens += result.PromptTokens
		output.CompletionTokens += result.CompletionTokens
		output.ThinkingTokens += result.ThinkingTokens
		output.DegradationCodes = append(output.DegradationCodes, result.DegradationCodes...)
		if !result.UsageAvailable {
			output.UsageAvailable = false
		}
	}
	return output, nil
}

func structuredMergeError(format string, args ...interface{}) error {
	return fmt.Errorf("structured summary merge: "+format+": %w", append(args, domain.ErrGenerationUnsupported)...)
}

func summaryLanguage(trans *domain.Transcription) string {
	if len(trans.Languages) == 1 && trans.Languages[0] != "auto" {
		return trans.Languages[0]
	}
	return ""
}

type summaryUtterance struct {
	Speaker *int   `json:"speaker"`
	Text    string `json:"text"`
	Words   []struct {
		Word string `json:"word"`
	} `json:"words"`
}

func summaryInputText(summaryType string, highStakes bool, content *domain.TranscriptionContent) string {
	text, ok := speakerAwareUtteranceText(content.Utterances, content.SpeakerMap)
	if !ok {
		return content.FullTranscript
	}
	if highStakes && summaryType != domain.SummaryTypeQAndA {
		return combinedSummaryTranscript(content.FullTranscript, text)
	}
	if summaryType != domain.SummaryTypeQAndA {
		return content.FullTranscript
	}
	return text
}

func combinedSummaryTranscript(fullTranscript, speakerText string) string {
	if strings.TrimSpace(fullTranscript) == "" {
		return "Speaker-labeled transcript:\n" + speakerText
	}
	combined := "Full transcript:\n" + fullTranscript + "\n\nSpeaker-labeled transcript for attribution:\n" + speakerText
	if len(combined) > maxTranscriptLen {
		return fullTranscript
	}
	return combined
}

func speakerAwareUtteranceText(raw json.RawMessage, speakerMap map[string]string) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var utterances []summaryUtterance
	if err := json.Unmarshal(raw, &utterances); err != nil || len(utterances) == 0 {
		return "", false
	}
	var builder strings.Builder
	wrote := false
	for _, utterance := range utterances {
		if utterance.Speaker == nil || *utterance.Speaker < 0 {
			continue
		}
		text := resolvedUtteranceText(utterance)
		if text == "" {
			continue
		}
		if wrote {
			builder.WriteByte('\n')
		}
		builder.WriteString("[")
		builder.WriteString(resolveSpeakerName(*utterance.Speaker, speakerMap))
		builder.WriteString("] ")
		builder.WriteString(text)
		wrote = true
	}
	return builder.String(), wrote
}

func resolvedUtteranceText(utterance summaryUtterance) string {
	if text := strings.TrimSpace(utterance.Text); text != "" {
		return text
	}
	words := make([]string, 0, len(utterance.Words))
	for _, word := range utterance.Words {
		if text := strings.TrimSpace(word.Word); text != "" {
			words = append(words, text)
		}
	}
	return strings.Join(words, " ")
}

func resolveSpeakerName(speaker int, speakerMap map[string]string) string {
	if name := sanitizeSpeakerName(speakerMap[strconv.Itoa(speaker)]); name != "" && !looksInstructionalSpeakerName(name) {
		return name
	}
	return fmt.Sprintf("Speaker %d", speaker)
}

func sanitizeSpeakerName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune("[]{}<>", r) || unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, name)
	trimmed := strings.Join(strings.Fields(cleaned), " ")
	if len([]rune(trimmed)) <= 80 {
		return trimmed
	}
	return string([]rune(trimmed)[:80])
}

func looksInstructionalSpeakerName(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range []string{"assistant:", "system:", "user:", "developer:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, phrase := range []string{"ignore instructions", "ignore previous", "disregard instructions"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func summarySourceFailureMessage(err error) string {
	if errors.Is(err, domain.ErrInvalidInput) {
		return "transcript contains no usable content"
	}
	return "transcript too large for supported summary processing"
}

// handleFailure marks a summary as failed via the SummaryService (best-effort).
func (w *SummarizeWorker) handleFailure(ctx context.Context, summaryID, msg string, logger *slog.Logger) {
	if err := w.summarySvc.HandleFailure(ctx, summaryID, msg); err != nil {
		logger.Warn("failed to mark summary as failed", "error", err)
	}
}
