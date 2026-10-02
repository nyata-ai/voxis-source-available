package domain

import "errors"

// Sentinel errors for domain operations
var (
	// ErrNotFound indicates the requested resource was not found
	ErrNotFound = errors.New("resource not found")

	// ErrUnauthorized indicates the user is not authenticated
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the user lacks permission
	ErrForbidden = errors.New("forbidden")

	// ErrInsufficientCredits indicates not enough credits for operation
	ErrInsufficientCredits = errors.New("insufficient credits")

	// ErrCreditsExpired indicates the balance lapsed past its expiry date.
	// Distinct from ErrInsufficientCredits: the user must buy more to restore
	// access, not merely top up a shortfall.
	ErrCreditsExpired = errors.New("credits expired")

	// ErrInvalidInput indicates the input failed validation
	ErrInvalidInput = errors.New("invalid input")

	// ErrConflict indicates a resource conflict (e.g., duplicate)
	ErrConflict = errors.New("resource conflict")

	// ErrRateLimited indicates the request was rejected due to rate limiting
	ErrRateLimited = errors.New("rate limited")

	// ErrInternal indicates an unexpected internal error
	ErrInternal = errors.New("internal error")

	// ErrContentBlocked indicates AI content was blocked by safety filters
	ErrContentBlocked = errors.New("content blocked by safety filters")

	// ErrGenerationIncomplete indicates an AI generation stopped before it
	// produced a complete result. Retrying the same request will not complete it.
	ErrGenerationIncomplete = errors.New("generation incomplete")

	// ErrGenerationUnsupported indicates an AI provider returned a completed
	// response that Voxis cannot safely use. Retrying the same request will not
	// change that response outcome.
	ErrGenerationUnsupported = errors.New("unsupported generation outcome")

	// ErrStructuredRequestRejected indicates the provider rejected a structured
	// generation request before producing content. The worker may safely retry
	// once through the legacy response path when the complete source fits.
	ErrStructuredRequestRejected = errors.New("structured generation request rejected")

	// ErrAudioUnavailable indicates the media row exists but active audio bytes
	// or encryption metadata are no longer available.
	ErrAudioUnavailable = errors.New("audio unavailable")

	// ErrRecordingTooShort indicates a stitched recording is below the minimum
	// storable media size. Deterministic for a given chunk set, so stitch jobs
	// must fail the session immediately instead of retrying.
	ErrRecordingTooShort = errors.New("recording too short")

	// ErrMediaTooLong indicates audio longer than the configured
	// MEDIA_MAX_DURATION. Deterministic for a given file, so jobs fail
	// immediately. Handlers map it to 422 media_too_long.
	ErrMediaTooLong = errors.New("media too long")

	// ErrModelBusy indicates the local model has no free slot within the
	// interactive wait. Handlers map it to 429 model_busy.
	ErrModelBusy = errors.New("model busy")

	// ErrInsufficientScratchSpace indicates the scratch filesystem cannot
	// hold the working files a media job needs.
	ErrInsufficientScratchSpace = errors.New("insufficient scratch space")

	// ErrStorageQuotaExceeded indicates that a bucket-backed storage write
	// would exceed the authenticated user's configured storage allowance.
	ErrStorageQuotaExceeded = errors.New("storage quota exceeded")

	// Authentication errors (use these for auth-specific failures)
	// Note: ErrUnauthorized is for general "not logged in" cases
	// These are for specific token validation failures
	ErrMissingToken = errors.New("missing authorization token")
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrTokenExpired = errors.New("token has expired")

	// Signup anti-abuse errors (registration hardening)
	// ErrDisposableEmail indicates registration was attempted with an email
	// address on a known disposable/temporary-inbox domain. Distinct from
	// ErrInvalidInput so handlers can surface a dedicated error code instead of
	// a generic validation message.
	ErrDisposableEmail = errors.New("disposable email address")
	// ErrTurnstileRequired indicates a Turnstile-protected registration request
	// arrived without a token, or the token failed Cloudflare Siteverify (or
	// Siteverify itself was unreachable — verification fails closed). Kept
	// separate from ErrInvalidInput for the same reason as ErrDisposableEmail.
	ErrTurnstileRequired = errors.New("turnstile verification required")

	// Privilege Recording errors
	// ErrPrivilegeReadBlocked is returned when a regular read path attempts to
	// access privilege content. Handlers should map this to 404 to avoid
	// leaking the existence of privilege media via differing error codes.
	ErrPrivilegeReadBlocked = errors.New("privilege content not accessible")
	// ErrPrivilegeOnly is returned when a regular write path (e.g., POST
	// /summaries) targets privilege media. Handlers map to 403.
	ErrPrivilegeOnly = errors.New("operation requires privilege workspace")
	// ErrPrivilegeStateInvalid is returned when an operation is attempted
	// against a media in the wrong privilege state.
	ErrPrivilegeStateInvalid = errors.New("privilege state does not allow this operation")

	// Examination record (BAP export) preconditions. All three map to 409:
	// the request is well-formed, but the transcription is not yet in a state
	// that can produce a verbatim-anchored examination record.
	// ErrExaminationSummaryMissing is returned when the transcription has no
	// completed Q&A summary to draw questions and answers from.
	ErrExaminationSummaryMissing = errors.New("examination record requires a completed Q&A summary")
	// ErrExaminationSummaryUnstructured is returned when the completed Q&A
	// summary predates structured output, or was degraded to the legacy prose
	// path, so it carries no citation-addressable items.
	ErrExaminationSummaryUnstructured = errors.New("examination record requires a structured Q&A summary")
	// ErrExaminationSourceMismatch is returned when the summary's source hash
	// no longer matches the transcript (a speaker was renamed after
	// generation), which makes every citation unresolvable.
	ErrExaminationSourceMismatch = errors.New("examination record citations no longer match the transcript")
)

// StorageQuotaExceededError carries stable, client-actionable quota details.
// It unwraps to ErrStorageQuotaExceeded so callers can use errors.Is.
type StorageQuotaExceededError struct {
	RequestedBytes int64
	UsedBytes      int64
	LimitBytes     int64
	RemainingBytes int64
}

func (e *StorageQuotaExceededError) Error() string { return ErrStorageQuotaExceeded.Error() }

func (e *StorageQuotaExceededError) Unwrap() error { return ErrStorageQuotaExceeded }
