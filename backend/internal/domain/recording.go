package domain

import (
	"fmt"
	"time"
)

// Recording session status constants.
const (
	RecordingStatusRecording   = "recording"
	RecordingStatusPaused      = "paused"
	RecordingStatusCompleting  = "completing"
	RecordingStatusCompleted   = "completed"
	RecordingStatusFailed      = "failed"
	RecordingStatusInterrupted = "interrupted"
	RecordingStatusAbandoned   = "abandoned"
)

// Recording MIME type constants.
const (
	RecordingMimeWebM = "audio/webm"
	RecordingMimeMP4  = "audio/mp4"
)

// RecordingCaptureSource identifies the browser capture source used for a session.
type RecordingCaptureSource string

const (
	// RecordingCaptureSourceMicrophone records only microphone audio.
	RecordingCaptureSourceMicrophone RecordingCaptureSource = "microphone"
	// RecordingCaptureSourceMixedAudio mixes microphone audio with a browser display-capture audio track.
	RecordingCaptureSourceMixedAudio RecordingCaptureSource = "mixed_audio"
)

// IsValid reports whether the capture source is supported.
func (s RecordingCaptureSource) IsValid() bool {
	return s == RecordingCaptureSourceMicrophone || s == RecordingCaptureSourceMixedAudio
}

// Recording limits.
const (
	// MaxChunksPerSession is a domain-level hard safety ceiling.
	// Service-level policy may enforce a lower per-session limit.
	MaxChunksPerSession = 2000
	MaxChunkSize        = 25 * 1024 * 1024 // 25 MB
	// MaxRecordingSessionBytes caps the total encoded browser audio retained
	// for one session. It matches the largest media artifact the platform can
	// store and keeps stitching's temporary disk requirements bounded.
	MaxRecordingSessionBytes = MaxMediaSize
	MaxInterruptedPerUser    = 3
	MaxMicrophoneLabelLength = 255
	MaxRecordingMimeLength   = 100
)

// WebM magic bytes: EBML header identifier.
var webmMagicBytes = []byte{0x1A, 0x45, 0xDF, 0xA3}

// RecordingSession represents an active or completed recording session.
type RecordingSession struct {
	ID              string
	OrganizationID  string
	UserID          string
	Status          string
	MimeType        string
	MicrophoneLabel string
	MediaID         string
	TotalDuration   float64
	RecordingMode   RecordingMode
	CaptureSource   RecordingCaptureSource
	LastChunkAt     *time.Time
	LastActivityAt  *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	// ChunkCount is derived from the chunk manifest, not stored on the session
	// row. Only read paths that surface a session to a client populate it; it is
	// zero everywhere else.
	ChunkCount int
}

// NewRecordingSession creates a new regular-mode session with validated fields.
// ID is left empty (assigned by the database).
func NewRecordingSession(orgID, userID, mimeType, micLabel string) (*RecordingSession, error) {
	return NewRecordingSessionWithMode(orgID, userID, mimeType, micLabel, RecordingModeRegular)
}

// NewRecordingSessionWithMode is like NewRecordingSession but allows the caller
// to set the recording mode (regular or privilege).
func NewRecordingSessionWithMode(orgID, userID, mimeType, micLabel string, mode RecordingMode) (*RecordingSession, error) {
	return NewRecordingSessionWithModeAndCaptureSource(orgID, userID, mimeType, micLabel, mode, RecordingCaptureSourceMicrophone)
}

// NewRecordingSessionWithModeAndCaptureSource creates a session with explicit mode and browser capture source.
func NewRecordingSessionWithModeAndCaptureSource(
	orgID, userID, mimeType, micLabel string,
	mode RecordingMode,
	captureSource RecordingCaptureSource,
) (*RecordingSession, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty: %w", ErrInvalidInput)
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty: %w", ErrInvalidInput)
	}
	if !isValidRecordingMime(mimeType) {
		return nil, fmt.Errorf("unsupported recording MIME type %q: %w", mimeType, ErrInvalidInput)
	}
	if len(micLabel) > MaxMicrophoneLabelLength {
		return nil, fmt.Errorf("microphone label exceeds %d characters: %w", MaxMicrophoneLabelLength, ErrInvalidInput)
	}
	if !mode.IsValid() {
		return nil, fmt.Errorf("invalid recording mode %q: %w", mode, ErrInvalidInput)
	}
	if captureSource == "" {
		captureSource = RecordingCaptureSourceMicrophone
	}
	if !captureSource.IsValid() {
		return nil, fmt.Errorf("invalid capture source %q: %w", captureSource, ErrInvalidInput)
	}

	return &RecordingSession{
		OrganizationID:  orgID,
		UserID:          userID,
		Status:          RecordingStatusRecording,
		MimeType:        mimeType,
		MicrophoneLabel: micLabel,
		RecordingMode:   mode,
		CaptureSource:   captureSource,
	}, nil
}

// Pause transitions from recording to paused.
func (s *RecordingSession) Pause() error {
	if s.Status != RecordingStatusRecording {
		return fmt.Errorf("cannot pause session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusPaused
	s.UpdatedAt = time.Now()
	return nil
}

// Resume transitions from paused to recording.
func (s *RecordingSession) Resume() error {
	if s.Status != RecordingStatusPaused {
		return fmt.Errorf("cannot resume session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusRecording
	s.UpdatedAt = time.Now()
	return nil
}

// Complete transitions from recording or paused to completing.
func (s *RecordingSession) Complete() error {
	if s.Status != RecordingStatusRecording && s.Status != RecordingStatusPaused {
		return fmt.Errorf("cannot complete session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusCompleting
	s.UpdatedAt = time.Now()
	return nil
}

// MarkInterrupted transitions an active session (recording or paused) to
// interrupted (crash detection). Paused is included because the
// one-active-session unique index covers it too: a stale paused session that is
// never interrupted locks the user out of starting a new recording.
func (s *RecordingSession) MarkInterrupted() error {
	if !s.IsActive() {
		return fmt.Errorf("cannot interrupt session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusInterrupted
	s.UpdatedAt = time.Now()
	return nil
}

// Release is the user-initiated form of MarkInterrupted: it hands an active
// session to the recovery flow immediately instead of waiting for the orphan
// sweep. A wrong-state call is a conflict (HTTP 409), not bad input.
func (s *RecordingSession) Release() error {
	if !s.IsActive() {
		return fmt.Errorf("cannot release session in %q state: %w", s.Status, ErrConflict)
	}
	return s.MarkInterrupted()
}

// Recover transitions from interrupted to completing (user recovery).
func (s *RecordingSession) Recover() error {
	if s.Status != RecordingStatusInterrupted {
		return fmt.Errorf("cannot recover session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusCompleting
	s.UpdatedAt = time.Now()
	return nil
}

// Abandon transitions from interrupted to abandoned (user discard).
func (s *RecordingSession) Abandon() error {
	if s.Status != RecordingStatusInterrupted {
		return fmt.Errorf("cannot abandon session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusAbandoned
	s.UpdatedAt = time.Now()
	return nil
}

// MarkCompleted transitions from completing to completed with a media ID.
func (s *RecordingSession) MarkCompleted(mediaID string) error {
	if s.Status != RecordingStatusCompleting {
		return fmt.Errorf("cannot mark completed session in %q state: %w", s.Status, ErrInvalidInput)
	}
	if mediaID == "" {
		return fmt.Errorf("media ID cannot be empty: %w", ErrInvalidInput)
	}
	s.Status = RecordingStatusCompleted
	s.MediaID = mediaID
	now := time.Now()
	s.CompletedAt = &now
	s.UpdatedAt = now
	return nil
}

// MarkFailed transitions from completing to failed.
func (s *RecordingSession) MarkFailed() error {
	if s.Status != RecordingStatusCompleting {
		return fmt.Errorf("cannot mark failed session in %q state: %w", s.Status, ErrInvalidInput)
	}
	s.Status = RecordingStatusFailed
	s.UpdatedAt = time.Now()
	return nil
}

// IsActive returns true if the session is in an active state (recording or paused).
func (s *RecordingSession) IsActive() bool {
	return s.Status == RecordingStatusRecording || s.Status == RecordingStatusPaused
}

// AcceptsChunks returns true if the session can receive new chunks.
// Interrupted sessions accept chunks because crash recovery flushes the client's
// leftover local chunks before calling recover; rejecting them would discard
// audio that only exists in the browser. Accepting a chunk never changes the
// session status, so the interrupted lifecycle is preserved.
func (s *RecordingSession) AcceptsChunks() bool {
	return s.IsActive() || s.Status == RecordingStatusInterrupted
}

// RecordingChunk represents a single uploaded chunk in the manifest.
type RecordingChunk struct {
	ID             string
	SessionID      string
	Seq            int
	StorageKey     string
	WrappedDEK     []byte
	WrappingNonce  []byte
	EncryptionAlgo string
	ChunkSize      int
	ChunkCount     int
	PlaintextSize  int64
	Checksum       string
	UploadedAt     time.Time
}

// NewRecordingChunk creates a new chunk with validated fields.
func NewRecordingChunk(sessionID string, seq int, storageKey string) (*RecordingChunk, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty: %w", ErrInvalidInput)
	}
	if seq < 0 {
		return nil, fmt.Errorf("sequence number cannot be negative: %w", ErrInvalidInput)
	}
	if seq >= MaxChunksPerSession {
		return nil, fmt.Errorf("sequence number %d exceeds maximum %d: %w", seq, MaxChunksPerSession-1, ErrInvalidInput)
	}
	if storageKey == "" {
		return nil, fmt.Errorf("storage key cannot be empty: %w", ErrInvalidInput)
	}

	return &RecordingChunk{
		SessionID:  sessionID,
		Seq:        seq,
		StorageKey: storageKey,
	}, nil
}

// SetEncryptionMeta sets the encryption metadata on a chunk after encryption.
func (c *RecordingChunk) SetEncryptionMeta(wrappedDEK, wrappingNonce []byte, algo string, chunkSize, chunkCount int, plaintextSize int64, checksum string) {
	c.WrappedDEK = wrappedDEK
	c.WrappingNonce = wrappingNonce
	c.EncryptionAlgo = algo
	c.ChunkSize = chunkSize
	c.ChunkCount = chunkCount
	c.PlaintextSize = plaintextSize
	c.Checksum = checksum
}

// ChunkStorageKey returns the GCS object key for a chunk.
// Format: orgs/{orgID}/recordings/{sessionID}/chunk-{NNN}.bin.enc
// Privilege sessions are namespaced under domain.PrivilegeStoragePrefix so
// their objects land in the shred bucket (see storage_key.go).
func ChunkStorageKey(orgID, sessionID string, seq int, mode RecordingMode) string {
	return applyPrivilegePrefix(
		fmt.Sprintf("orgs/%s/recordings/%s/chunk-%03d.bin.enc", orgID, sessionID, seq), mode)
}

// RecordingChunksPrefix returns the GCS prefix for all chunks of a session.
// It must be built with the same mode used to write the chunks, or the sweep
// looks in the wrong bucket and namespace.
func RecordingChunksPrefix(orgID, sessionID string, mode RecordingMode) string {
	return applyPrivilegePrefix(fmt.Sprintf("orgs/%s/recordings/%s/", orgID, sessionID), mode)
}

// isValidRecordingMime checks if the MIME type is valid for recording.
func isValidRecordingMime(mimeType string) bool {
	return mimeType == RecordingMimeWebM || mimeType == RecordingMimeMP4
}

// IsWebMChunk checks if a chunk starts with WebM magic bytes.
func IsWebMChunk(data []byte) bool {
	if len(data) < len(webmMagicBytes) {
		return false
	}
	for i, b := range webmMagicBytes {
		if data[i] != b {
			return false
		}
	}
	return true
}

// IsMP4Chunk checks if a chunk starts with MP4 ftyp box signature.
// Bytes 4-7 should be "ftyp".
func IsMP4Chunk(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	return data[4] == 'f' && data[5] == 't' && data[6] == 'y' && data[7] == 'p'
}

// ValidateFirstChunk checks that the first chunk (seq=0) has correct magic bytes
// for the session's MIME type.
func ValidateFirstChunk(mimeType string, data []byte) error {
	switch mimeType {
	case RecordingMimeWebM:
		if !IsWebMChunk(data) {
			return fmt.Errorf("first chunk missing WebM magic bytes: %w", ErrInvalidInput)
		}
	case RecordingMimeMP4:
		if !IsMP4Chunk(data) {
			return fmt.Errorf("first chunk missing MP4 ftyp signature: %w", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("unknown MIME type %q: %w", mimeType, ErrInvalidInput)
	}
	return nil
}
