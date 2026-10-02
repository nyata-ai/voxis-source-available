package speechmatics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// meliaModel is the pinned Speechmatics model. Melia 1 is a single unified
// multilingual model, so the transcription language is always "multi" and
// hints only bias detection.
const meliaModel = "melia-1"

// Timeouts. The multipart submission carries the whole audio file, so it gets a
// generous deadline; status/delete calls are small and must fail fast rather
// than stalling a poll cycle.
const (
	submitTimeout    = 30 * time.Minute
	shortCallTimeout = 30 * time.Second
	// transcriptFetchTimeout covers a large json-v2 body.
	transcriptFetchTimeout = 2 * time.Minute
)

// Client-side rate limits, comfortably under the account-wide Speechmatics
// limits (10 job POST/s, 50 GET/s) so a burst of workers cannot spend the whole
// account budget.
//
// These limiters are PER PROCESS, not per account: two API replicas each get a
// full budget and can together reach 10 POST/s and 50 GET/s — exactly the
// account ceiling, with no headroom left. The halved rates buy that headroom
// for a two-replica deployment; a larger fleet needs either lower values here
// or a shared limiter, and would otherwise surface as 429s that the workers
// retry rather than as lost work.
const (
	postRatePerSecond = 5
	getRatePerSecond  = 25
)

// Response body caps. Errors are tiny; a json-v2 transcript for a 60-minute
// chunk is a few MB, and the cap refuses a pathological body rather than
// buffering it.
const (
	maxErrorBodyBytes      = 4 << 10
	maxJSONBodyBytes       = 1 << 20
	maxTranscriptBodyBytes = 128 << 20
)

// Client implements port.TranscriptionProvider using the Speechmatics batch API.
type Client struct {
	cfg           Config
	httpClient    *http.Client
	spoolRoot     string
	spoolDir      string
	maxSpoolBytes int64
	postLimiter   *rate.Limiter
	getLimiter    *rate.Limiter
	logger        *slog.Logger

	// mu guards pending, which carries the original filename and content type
	// from Upload to Submit. The port hands them to Upload but Speechmatics
	// needs them on the Submit multipart part, and the token between the two
	// calls is a plain string that gets persisted. Both calls happen inside one
	// worker job, so an entry lives for seconds.
	mu      sync.Mutex
	pending map[string]uploadMeta
}

// uploadMeta is what Upload learns and Submit needs.
type uploadMeta struct {
	filename    string
	contentType string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client (used by tests).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithLogger sets a custom structured logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// WithSpoolDir overrides the staging directory (used by tests). Production
// leaves it unset so the constructor mints an unpredictable per-process one.
func WithSpoolDir(dir string) Option {
	return func(c *Client) { c.spoolDir = dir }
}

// withSpoolRoot overrides the directory the per-process spool dir is created
// in and swept from. Unexported: production always uses os.TempDir().
func withSpoolRoot(dir string) Option {
	return func(c *Client) { c.spoolRoot = dir }
}

// withMaxSpoolBytes lowers the staging ceiling so tests can exercise it without
// writing the real 900 MB to disk. Unexported: production always uses
// maxSpoolBytes.
func withMaxSpoolBytes(limit int64) Option {
	return func(c *Client) { c.maxSpoolBytes = limit }
}

// NewClient creates a Speechmatics client from cfg. It mints this process's own
// staging directory and sweeps the ones earlier processes left behind.
func NewClient(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeSaaS
	}
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.Mode == ModeSaaS && cfg.APIKey == "" {
		return nil, fmt.Errorf("speechmatics: API key cannot be empty in saas mode: %w", domain.ErrInvalidInput)
	}

	c := &Client{
		cfg: cfg,
		// No client-level Timeout: each call sets its own context deadline, so
		// a 30-minute upload and a 30-second status probe share one client.
		httpClient:    &http.Client{},
		spoolRoot:     os.TempDir(),
		maxSpoolBytes: maxSpoolBytes,
		postLimiter:   rate.NewLimiter(postRatePerSecond, postRatePerSecond),
		getLimiter:    rate.NewLimiter(getRatePerSecond, getRatePerSecond),
		logger:        slog.Default(),
		pending:       make(map[string]uploadMeta),
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.spoolDir == "" {
		dir, err := os.MkdirTemp(c.spoolRoot, spoolDirPrefix)
		if err != nil {
			return nil, fmt.Errorf("speechmatics: create spool dir: %w", err)
		}
		c.spoolDir = dir
	}

	sweepStaleSpoolDirs(c.spoolRoot, c.spoolDir, staleSpoolDirMaxAge, c.logger)
	return c, nil
}

// rememberUpload records the port's filename and content type against a staged
// path so Submit can put them on the multipart part.
func (c *Client) rememberUpload(path string, meta uploadMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[path] = meta
}

// takeUpload removes and returns the metadata staged for path. A miss returns
// the zero value: Submit then falls back to a generic part header rather than
// refusing a submission over missing cosmetics.
func (c *Client) takeUpload(path string) uploadMeta {
	c.mu.Lock()
	defer c.mu.Unlock()
	meta := c.pending[path]
	delete(c.pending, path)
	return meta
}

// Upload stages the audio locally and returns an opaque token for Submit.
//
// Speechmatics has no separate upload endpoint — the audio goes in the same
// multipart request as the job config — so nothing leaves this process here.
// The staged plaintext is removed by Submit on every exit path, and by the
// constructor's sweep if the process dies in between.
func (c *Client) Upload(_ context.Context, src io.Reader, filename, contentType string) (string, error) {
	path, err := spoolFile(c.spoolDir, src, c.maxSpoolBytes, c.logger)
	if err != nil {
		return "", err
	}
	// Speechmatics infers the container format from the part's filename and
	// Content-Type, so both have to survive the Upload -> Submit hop.
	c.rememberUpload(path, uploadMeta{filename: filename, contentType: contentType})
	return spoolToken(path), nil
}

// Submit performs the multipart POST /v2/jobs and returns the provider job id.
// req.AudioURL must be the token returned by Upload.
func (c *Client) Submit(ctx context.Context, req port.TranscriptionRequest) (string, error) {
	path, err := parseSpoolToken(req.AudioURL, c.spoolDir)
	if err != nil {
		return "", err
	}
	// The staged plaintext and its metadata die with this call, success or
	// failure. takeUpload runs first so no error path can leak the entry.
	meta := c.takeUpload(path)
	defer removeSpoolFile(path, c.logger)

	// Melia 1 has no custom-dictionary feature yet: vocabulary packs cannot be
	// honored on this provider. Warn loudly rather than dropping them silently
	// — the transcript will not be biased toward the user's terms.
	if len(req.Vocabulary) > 0 && c.logger != nil {
		c.logger.Warn("speechmatics does not support custom vocabulary; dropping terms",
			"term_count", len(req.Vocabulary), "reference", req.Reference)
	}

	configJSON, err := c.buildJobConfig(req)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, submitTimeout)
	defer cancel()

	if waitErr := c.postLimiter.Wait(ctx); waitErr != nil {
		return "", fmt.Errorf("speechmatics: rate limiter: %w", waitErr)
	}

	httpReq, err := newMultipartJobRequest(ctx, c.cfg.APIURL+"/v2/jobs", path, meta, configJSON)
	if err != nil {
		return "", err
	}
	c.authorize(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// The body may already have been sent: the remote job could exist with
		// no local record. Accepted risk — Speechmatics auto-deletes after
		// 7 days, and tracking.reference makes it identifiable in the console.
		c.logger.Warn("Speechmatics submission outcome unknown; a remote job may be orphaned",
			"reference", req.Reference, "error", err)
		return "", fmt.Errorf("speechmatics: submit: %w", err)
	}
	defer closeBody(resp)

	if err := c.checkStatus(resp); err != nil {
		return "", err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(resp.Body, maxJSONBodyBytes, &result); err != nil {
		return "", fmt.Errorf("speechmatics: decode submit response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("speechmatics: submit response carried no job id: %w", domain.ErrInternal)
	}

	c.logger.Info("submitted transcription to Speechmatics", "speechmatics_job_id", result.ID)
	return result.ID, nil
}

// buildJobConfig renders the Speechmatics job configuration for req.
func (c *Client) buildJobConfig(req port.TranscriptionRequest) ([]byte, error) {
	hints, err := MapLanguageHints(req.Languages)
	if err != nil {
		return nil, err
	}
	if dropped := DroppedLanguageHints(req.Languages); len(dropped) > 0 && c.logger != nil {
		c.logger.Warn("speechmatics does not support some requested language hints; submitting without them",
			"dropped_languages", dropped, "reference", req.Reference)
	}
	if len(hints) == 0 {
		hints = c.cfg.LanguageHints
	}

	transcriptionConfig := map[string]interface{}{
		"model":    meliaModel,
		"language": "multi",
	}
	if req.Diarization {
		transcriptionConfig["diarization"] = "speaker"
		// Diarization tuning is meaningless without diarization itself, so the
		// block is only ever emitted alongside "diarization": "speaker".
		if diarizationConfig := c.buildSpeakerDiarizationConfig(req.ExpectedSpeakers); diarizationConfig != nil {
			transcriptionConfig["speaker_diarization_config"] = diarizationConfig
		}
		c.warnUnappliedSpeakerCount(req)
	}
	if len(hints) > 0 {
		transcriptionConfig["language_hints"] = hints
	}

	body := map[string]interface{}{
		"type":                 "transcription",
		"transcription_config": transcriptionConfig,
	}
	if req.Reference != "" {
		body["tracking"] = map[string]string{"reference": req.Reference}
	}
	if req.CallbackEnabled && req.CallbackURL != "" {
		// The notification is a WAKE-UP SIGNAL only, so "contents" is
		// deliberately empty: Speechmatics posts an empty body with the job id
		// and status in the query string, and no transcript or audio is ever
		// pushed to us. auth_headers is likewise unset — the shared secret
		// travels in the URL path, exactly like the Gladia callback.
		body["notification_config"] = []map[string]interface{}{{
			"url":      req.CallbackURL,
			"contents": []string{},
		}}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("speechmatics: marshal job config: %w", err)
	}
	return encoded, nil
}

// fewSpeakerSensitivity is the speaker_sensitivity used when the submitter
// declared one or two speakers.
//
// The batch API has no min/max speaker knob, so a declared count can only be
// honored by tuning sensitivity. A sweep of the tuned deployment default (0.4)
// showed that going lower merges genuinely distinct speakers once three or more
// voices are present — which is why the global value is never set below 0.4.
// With at most two real voices there is nothing left to merge, so the low value
// is safe there and stops the over-splitting that produced ~10 labels for a
// two-person interview.
const fewSpeakerSensitivity = 0.2

// maxFewSpeakers is the largest declared speaker count that gets the low
// sensitivity. Above it the configured deployment behavior is left alone.
const maxFewSpeakers = 2

// buildSpeakerDiarizationConfig renders the optional speaker_diarization_config
// block, containing only the fields that are actually set. Returns nil when
// nothing applies, so the caller can omit the block and let provider defaults
// apply.
//
// expectedSpeakers is the submitter's declared speaker count (0 = auto-detect).
// A declaration of 1 or 2 overrides the deployment's configured sensitivity and
// forces the block to be emitted even when no knob is configured; the
// configured prefer_current_speaker is preserved either way.
func (c *Client) buildSpeakerDiarizationConfig(expectedSpeakers int) map[string]interface{} {
	fewSpeakers := expectedSpeakers > 0 && expectedSpeakers <= maxFewSpeakers
	if !fewSpeakers && c.cfg.SpeakerSensitivity == nil && c.cfg.PreferCurrentSpeaker == nil {
		return nil
	}
	cfg := make(map[string]interface{}, 2)
	switch {
	case fewSpeakers:
		cfg["speaker_sensitivity"] = fewSpeakerSensitivity
	case c.cfg.SpeakerSensitivity != nil:
		cfg["speaker_sensitivity"] = *c.cfg.SpeakerSensitivity
	}
	if c.cfg.PreferCurrentSpeaker != nil {
		cfg["prefer_current_speaker"] = *c.cfg.PreferCurrentSpeaker
	}
	return cfg
}

// warnUnappliedSpeakerCount reports, once per submission, that a declared count
// above maxFewSpeakers changed nothing about this job.
//
// The batch API exposes no min/max speaker knob, so a declaration of 3 or more
// is simply unusable here — the user picked a number, paid for the job and got
// provider-default diarization. Dropping that silently makes a "why did it find
// eight speakers when I said four?" report impossible to answer from the logs.
func (c *Client) warnUnappliedSpeakerCount(req port.TranscriptionRequest) {
	if req.ExpectedSpeakers <= maxFewSpeakers || c.logger == nil {
		return
	}
	c.logger.Warn("speechmatics cannot constrain diarization to a declared speaker count; using provider defaults",
		"expected_speakers", req.ExpectedSpeakers,
		"max_applied_speakers", maxFewSpeakers,
		"reference", req.Reference,
	)
}

// GetStatus polls GET /v2/jobs/{id} and, once the job is done, fetches and maps
// the json-v2 transcript.
func (c *Client) GetStatus(ctx context.Context, jobID string) (*port.TranscriptionResult, error) {
	if jobID == "" {
		return nil, fmt.Errorf("speechmatics: empty job ID: %w", domain.ErrInvalidInput)
	}

	job, err := c.fetchJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return c.resultForJob(ctx, jobID, job)
}

// maxServerWaitSeconds bounds the server-side wait we ask for. Speechmatics
// caps it on its side too; asking for more only risks a rejected parameter.
const (
	minServerWaitSeconds = 1
	maxServerWaitSeconds = 60
)

// WaitForResult is GetStatus with a server-side wait: GET /v2/jobs/{id}?wait=N
// blocks up to N seconds until the job reaches a terminal state and answers 200
// with the job either way. Melia finishes most files in a few seconds, so one
// bounded wait right after submission usually completes the job immediately
// instead of leaving it to the next periodic poll.
//
// It is deliberately NOT part of port.TranscriptionProvider: it is a
// Speechmatics-only optimization, and callers reach it by type-assertion so a
// provider without it simply falls back to polling.
func (c *Client) WaitForResult(ctx context.Context, jobID string, maxWait time.Duration) (*port.TranscriptionResult, error) {
	if jobID == "" {
		return nil, fmt.Errorf("speechmatics: empty job ID: %w", domain.ErrInvalidInput)
	}

	waitSeconds := int(maxWait / time.Second)
	if waitSeconds < minServerWaitSeconds {
		waitSeconds = minServerWaitSeconds
	}
	if waitSeconds > maxServerWaitSeconds {
		waitSeconds = maxServerWaitSeconds
	}

	// The call is expected to block for the whole wait, so the deadline has to
	// cover it plus the usual round-trip budget.
	job, err := c.fetchJobWaiting(ctx, jobID, waitSeconds,
		time.Duration(waitSeconds)*time.Second+shortCallTimeout)
	if err != nil {
		return nil, err
	}
	return c.resultForJob(ctx, jobID, job)
}

// resultForJob maps one fetched job to a port result, fetching the transcript
// when the job is done. GetStatus and WaitForResult share it so the terminal
// classification below exists exactly once.
func (c *Client) resultForJob(ctx context.Context, jobID string, job jobDetails) (*port.TranscriptionResult, error) {
	// Live-observed statuses (2026-08-30): only "running" and "done". The
	// others below are documented but UNVERIFIED against the real API —
	// "accepted"/"queued" are harmless if wrong (they fall through to the
	// default branch, which retries), but "rejected"/"expired" are TERMINAL
	// and delete the provider's copy of the audio. Confirm both against a real
	// rejected job before relying on them.
	switch job.Status {
	case "running", "accepted", "queued":
		return &port.TranscriptionResult{Status: "processing"}, nil
	case "done":
		return c.fetchTranscript(ctx, jobID)
	case "rejected", "expired":
		return &port.TranscriptionResult{
			Status:       "error",
			ErrorCode:    http.StatusUnprocessableEntity,
			ErrorMessage: jobErrorMessage(job.Status),
		}, nil
	default:
		// Pass the raw status through instead of calling it an error. "error" is
		// TERMINAL to the poller: it
		// marks the transcription failed and then deletes the provider job,
		// which at that point holds the only copy of the audio. Speechmatics
		// already documents statuses this switch does not name ("deleted"),
		// and a vendor may add more. An unrecognized status lands in the
		// poller's own default branch, which logs and leaves the row alone for
		// the next cycle — the only non-destructive way to be wrong.
		c.logger.Warn("unrecognized Speechmatics job status; leaving the job untouched",
			"speechmatics_job_id", jobID, "provider_status", job.Status)
		return &port.TranscriptionResult{Status: job.Status}, nil
	}
}

// jobDetails is the subset of GET /v2/jobs/{id} we read.
type jobDetails struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// jobErrorMessage returns a stable, provider-safe error category.
func jobErrorMessage(status string) string {
	if status == "expired" {
		return "Speechmatics job expired"
	}
	return "Speechmatics job rejected"
}

// noWaitQuery disables Speechmatics' server-side long-poll on a job-resource
// GET.
//
// Since 2026-08-07 GET /v2/jobs/{id} and GET /v2/jobs/{id}/transcript block
// server-side for up to 2s waiting for a terminal state unless wait=0 is
// passed, and the vendor says that default will grow. PollSpeechmaticsWorker
// walks up to 100 rows SERIALLY per cycle (parents then segments), so without
// this every stale row would silently sleep inside the provider instead of
// returning immediately — turning a fast poll cycle into one bounded only by
// how many rows happen to still be running.
const noWaitQuery = "wait=0"

// fetchJob retrieves job metadata.
func (c *Client) fetchJob(ctx context.Context, jobID string) (jobDetails, error) {
	resp, err := c.doShortGet(ctx, "/v2/jobs/"+url.PathEscape(jobID)+"?"+noWaitQuery, shortCallTimeout)
	if err != nil {
		return jobDetails{}, err
	}
	defer closeBody(resp)

	return c.decodeJobEnvelope(resp, jobID)
}

// fetchJobWaiting is fetchJob with the server-side ?wait=N parameter, which
// makes the GET block until the job reaches a terminal state. It is a sibling
// rather than a parameter on fetchJob so the plain status path keeps its own
// short deadline and its own URL untouched.
func (c *Client) fetchJobWaiting(
	ctx context.Context, jobID string, waitSeconds int, timeout time.Duration,
) (jobDetails, error) {
	path := fmt.Sprintf("/v2/jobs/%s?wait=%d", url.PathEscape(jobID), waitSeconds)
	resp, err := c.doShortGet(ctx, path, timeout)
	if err != nil {
		return jobDetails{}, err
	}
	defer closeBody(resp)

	return c.decodeJobEnvelope(resp, jobID)
}

// decodeJobEnvelope validates the response status and decodes the {"job": ...}
// envelope both job fetches share.
func (c *Client) decodeJobEnvelope(resp *http.Response, jobID string) (jobDetails, error) {
	if err := c.checkStatus(resp); err != nil {
		return jobDetails{}, err
	}

	var payload struct {
		Job jobDetails `json:"job"`
	}
	if err := decodeJSON(resp.Body, maxJSONBodyBytes, &payload); err != nil {
		return jobDetails{}, fmt.Errorf("speechmatics: decode job status: %w", err)
	}
	// A body that carried no "job" envelope, or one with no status, decodes
	// cleanly into the zero value. Reporting that as a status would let a
	// proxy error page or a truncated 200 drive a terminal decision, so it is
	// a FETCH failure instead: the poller logs it and retries next cycle.
	if payload.Job.Status == "" {
		return jobDetails{}, fmt.Errorf(
			"speechmatics: job %s status response carried no job status: %w", jobID, domain.ErrInternal)
	}
	return payload.Job, nil
}

// fetchTranscript retrieves and maps the json-v2 transcript of a done job.
func (c *Client) fetchTranscript(ctx context.Context, jobID string) (*port.TranscriptionResult, error) {
	resp, err := c.doShortGet(ctx,
		"/v2/jobs/"+url.PathEscape(jobID)+"/transcript?format=json-v2&"+noWaitQuery, transcriptFetchTimeout)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)

	if statusErr := c.checkStatus(resp); statusErr != nil {
		return nil, statusErr
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTranscriptBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("speechmatics: read transcript: %w", err)
	}
	return MapTranscript(raw)
}

// Delete removes the job and its audio from Speechmatics. 404 means the job is
// already gone, which is the outcome the caller wants.
func (c *Client) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("speechmatics: empty job ID: %w", domain.ErrInvalidInput)
	}

	ctx, cancel := context.WithTimeout(ctx, shortCallTimeout)
	defer cancel()

	// DELETE spends the GET budget, not the POST one. Speechmatics rate-limits
	// job CREATION separately from every other job-resource call, so a deletion
	// backlog draining at 25/s must not eat the submission budget and stall new
	// transcriptions behind cleanup work.
	if err := c.getLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("speechmatics: rate limiter: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodDelete, "/v2/jobs/"+url.PathEscape(jobID))
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("speechmatics: delete: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return c.checkStatus(resp)
}

// ConfirmDeleted verifies the job is gone:
// 404 confirms deletion (nil), a 2xx means it still exists (ErrConflict so the
// caller retries), anything else surfaces as-is.
func (c *Client) ConfirmDeleted(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("speechmatics: empty job ID: %w", domain.ErrInvalidInput)
	}

	resp, err := c.doShortGet(ctx, "/v2/jobs/"+url.PathEscape(jobID)+"?"+noWaitQuery, shortCallTimeout)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil // deletion confirmed
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return fmt.Errorf("speechmatics: job %s still exists after delete: %w", jobID, domain.ErrConflict)
	}
	return c.checkStatus(resp)
}

// CheckAuth verifies the configured credential against the configured region
// with the cheapest authenticated call the API offers: one page of the job
// list. It is a STARTUP probe, not part of the port — a wrong region is
// otherwise invisible until the first real submission fails.
//
// It exists because eu2.asr.api.speechmatics.com answers a valid key with an
// nginx HTTP 401 (2026-08-30 live testing), which is indistinguishable from a
// bad key at the call site and would otherwise surface as every transcription
// failing hours after a deploy.
//
// A rejected credential returns domain.ErrUnauthorized; the caller decides
// whether that is fatal. Every other failure (network, 5xx) is returned as-is
// so the caller can treat it as inconclusive rather than as a verdict.
func (c *Client) CheckAuth(ctx context.Context) error {
	resp, err := c.doShortGet(ctx, "/v2/jobs?limit=1", shortCallTimeout)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	return c.checkStatus(resp)
}

// Compile-time interface compliance check.
var _ port.TranscriptionProvider = (*Client)(nil)
