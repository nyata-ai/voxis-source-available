package domain

import (
	"fmt"
	"mime"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// Media status constants.
const (
	MediaStatusPending    = "pending"
	MediaStatusEncrypting = "encrypting"
	MediaStatusReady      = "ready"
	MediaStatusFailed     = "failed"
	MediaStatusDeleted    = "deleted"
)

// Scan status constants (for malware scanning).
const (
	ScanStatusPending  = "scan_pending"
	ScanStatusClean    = "scan_clean"
	ScanStatusInfected = "scan_infected"
	ScanStatusError    = "scan_error"
	ScanStatusSkipped  = "scan_skipped"
)

// MaxMediaSize is the maximum allowed upload size (1 GB).
const MaxMediaSize = 1024 * 1024 * 1024

// MinMediaSize is the minimum allowed upload size (1 KB) to reject empty/trivial uploads.
const MinMediaSize = 1024

// MaxTitleLength is the maximum allowed title length.
const MaxTitleLength = 255

// MaxDescriptionLength is the maximum allowed description length.
const MaxDescriptionLength = 1000

// allowedAudioTypes defines the set of accepted audio MIME types.
var allowedAudioTypes = map[string]bool{
	"audio/mpeg":  true,
	"audio/wav":   true,
	"audio/ogg":   true,
	"audio/flac":  true,
	"audio/mp4":   true,
	"audio/webm":  true,
	"audio/x-m4a": true,
	"audio/aac":   true,
	"audio/x-wav": true,
	"audio/opus":  true,
}

// IsAllowedAudioType reports whether the given content type is an allowed audio type.
// It handles MIME parameters (e.g., "audio/webm; codecs=opus") by extracting just the media type.
func IsAllowedAudioType(ct string) bool {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return allowedAudioTypes[mediaType]
}

// audioTypeAliases maps MIME types reported by browsers and operating systems
// onto the canonical types this service accepts. Windows registers .aac as
// audio/vnd.dlna.adts, which Chrome and Edge send verbatim.
var audioTypeAliases = map[string]string{
	"audio/vnd.dlna.adts": "audio/aac",
	"audio/x-aac":         "audio/aac",
	"audio/x-flac":        "audio/flac",
	"audio/mp3":           "audio/mpeg",
	"audio/x-mpeg":        "audio/mpeg",
	"audio/wave":          "audio/wav",
	"audio/vnd.wave":      "audio/wav",
	"audio/x-opus+ogg":    "audio/opus",
}

// NormalizeAudioContentType maps a declared content type to its canonical
// equivalent and drops MIME parameters. Unknown values remain unchanged so
// validation can report the original declaration.
func NormalizeAudioContentType(ct string) string {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ct
	}
	if canonical, ok := audioTypeAliases[mediaType]; ok {
		return canonical
	}
	if allowedAudioTypes[mediaType] {
		return mediaType
	}
	return ct
}

// knownNonAudioExtensions rejects files with extensions known to be non-audio.
// Unknown extensions pass through (handled by magic bytes and ffprobe).
var knownNonAudioExtensions = map[string]bool{
	".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".com": true,
	".msi": true, ".scr": true, ".pif": true, ".vbs": true, ".js": true,
	".ps1": true, ".sh": true, ".bin": true, ".iso": true, ".zip": true,
	".rar": true, ".7z": true, ".tar": true, ".gz": true, ".pdf": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".html": true, ".htm": true, ".php": true, ".py": true,
}

// extensionToMIME maps audio file extensions to expected MIME types.
var extensionToMIME = map[string][]string{
	".mp3":  {"audio/mpeg"},
	".wav":  {"audio/wav", "audio/x-wav"},
	".ogg":  {"audio/ogg"},
	".flac": {"audio/flac"},
	".m4a":  {"audio/mp4", "audio/x-m4a"},
	".mp4":  {"audio/mp4"},
	".webm": {"audio/webm"},
	".aac":  {"audio/aac"},
	".opus": {"audio/opus", "audio/ogg"},
	".weba": {"audio/webm"},
}

// ValidateExtensionMIME checks that the file extension is consistent with the
// declared MIME type. Rejects known non-audio extensions. For known audio
// extensions, validates the MIME type matches. Unknown extensions pass through.
func ValidateExtensionMIME(filename, contentType string) error {
	ext := strings.ToLower(path.Ext(filename))
	if ext == "" {
		return nil // no extension — let other checks handle it
	}

	// Block known non-audio extensions regardless of MIME type
	if knownNonAudioExtensions[ext] {
		return fmt.Errorf("non-audio file extension %q: %w", ext, ErrInvalidInput)
	}

	expected, known := extensionToMIME[ext]
	if !known {
		return nil // unknown extension — let magic bytes and ffprobe handle it
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid content type: %w", ErrInvalidInput)
	}

	for _, e := range expected {
		if e == mediaType {
			return nil
		}
	}
	return fmt.Errorf("extension %q does not match content type %q: %w", ext, mediaType, ErrInvalidInput)
}

// MinSignatureBytes is the minimum bytes needed for magic byte validation.
const MinSignatureBytes = 12

// ValidateAudioSignature checks that the first bytes of data match the expected
// file signature for the given content type. This prevents disguised files
// (e.g., an ELF binary with audio/mpeg Content-Type) from passing upload validation.
// The data slice should be at least MinSignatureBytes long.
func ValidateAudioSignature(ct string, data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("file too short for signature check: %w", ErrInvalidInput)
	}

	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return fmt.Errorf("invalid content type: %w", ErrInvalidInput)
	}

	switch mediaType {
	case "audio/wav", "audio/x-wav":
		return checkWAVSignature(data)
	case "audio/mpeg":
		return checkMP3Signature(data)
	case "audio/flac":
		return checkFLACSignature(data)
	case "audio/ogg", "audio/opus":
		return checkOGGSignature(data)
	case "audio/webm":
		// Reuse IsWebMChunk from recording.go (DRY — same EBML magic bytes)
		if !IsWebMChunk(data) {
			return fmt.Errorf("missing EBML header: %w", ErrInvalidInput)
		}
		return nil
	case "audio/mp4", "audio/x-m4a":
		// Reuse IsMP4Chunk from recording.go (DRY — same ftyp box check)
		if !IsMP4Chunk(data) {
			return fmt.Errorf("missing ftyp box: %w", ErrInvalidInput)
		}
		return nil
	case "audio/aac":
		return checkAACSignature(data)
	default:
		return fmt.Errorf("no signature check for %q: %w", mediaType, ErrInvalidInput)
	}
}

func checkWAVSignature(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("WAV file too short: %w", ErrInvalidInput)
	}
	if data[0] != 'R' || data[1] != 'I' || data[2] != 'F' || data[3] != 'F' {
		return fmt.Errorf("missing RIFF header: %w", ErrInvalidInput)
	}
	if data[8] != 'W' || data[9] != 'A' || data[10] != 'V' || data[11] != 'E' {
		return fmt.Errorf("missing WAVE identifier: %w", ErrInvalidInput)
	}
	return nil
}

func checkMP3Signature(data []byte) error {
	// ID3v2 tag
	if len(data) >= 3 && data[0] == 'I' && data[1] == 'D' && data[2] == '3' {
		return nil
	}
	// MPEG sync word: 11-bit sync (0xFFE0). This intentionally overlaps with
	// AAC ADTS (which also uses 0xFFF sync). The Content-Type distinguishes them.
	// Both are valid MPEG audio frames — this check prevents non-MPEG binaries.
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
		return nil
	}
	return fmt.Errorf("missing MP3 signature (ID3 or sync word): %w", ErrInvalidInput)
}

func checkFLACSignature(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("FLAC file too short: %w", ErrInvalidInput)
	}
	if data[0] != 'f' || data[1] != 'L' || data[2] != 'a' || data[3] != 'C' {
		return fmt.Errorf("missing fLaC signature: %w", ErrInvalidInput)
	}
	return nil
}

func checkOGGSignature(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("OGG file too short: %w", ErrInvalidInput)
	}
	if data[0] != 'O' || data[1] != 'g' || data[2] != 'g' || data[3] != 'S' {
		return fmt.Errorf("missing OggS signature: %w", ErrInvalidInput)
	}
	return nil
}

func checkAACSignature(data []byte) error {
	// AAC in MP4 container (ftyp box) — reuse IsMP4Chunk
	if IsMP4Chunk(data) {
		return nil
	}
	// Raw ADTS sync word: 12-bit sync (0xFFF), then ID bit, then layer=00.
	// Check: byte[0]=0xFF, byte[1] bits 7-4=0xF, bits 2-1=00 -> mask 0xF6, expect 0xF0.
	if len(data) >= 2 && data[0] == 0xFF && (data[1]&0xF6) == 0xF0 {
		return nil
	}
	return fmt.Errorf("missing AAC signature (ftyp or ADTS): %w", ErrInvalidInput)
}

// Media represents an uploaded audio file.
type Media struct {
	ID             string
	OrganizationID string
	CreatedBy      string
	Filename       string
	Title          string
	Description    string
	ContentType    string
	Size           int64
	Duration       float64
	Status         string
	StorageKey     string
	WrappedDEK     []byte
	WrappingNonce  []byte
	EncryptionAlgo string
	ChunkSize      int
	ChunkCount     int
	FileHash       string
	AudioMetadata  []byte
	AudioDeletedAt *time.Time
	ScanStatus     string

	// RecordingMode remains only while common recording paths accept a mode argument.
	RecordingMode RecordingMode

	CreatedAt time.Time
	UpdatedAt time.Time

	// LatestTranscriptionID is populated by the service layer (not persisted).
	// Contains the ID of the most recent completed transcription, if any.
	LatestTranscriptionID string
}

// NewMedia creates a new Media with validated fields.
// ID is left empty (assigned by the database).
func NewMedia(orgID, filename, contentType string, size int64) (*Media, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty: %w", ErrInvalidInput)
	}
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty: %w", ErrInvalidInput)
	}
	if !IsAllowedAudioType(contentType) {
		return nil, fmt.Errorf("unsupported content type: %w", ErrInvalidInput)
	}
	if size < MinMediaSize || size > MaxMediaSize {
		return nil, fmt.Errorf("size must be between %d bytes and %d MB: %w", MinMediaSize, MaxMediaSize/(1024*1024), ErrInvalidInput)
	}

	return &Media{
		OrganizationID: orgID,
		Filename:       filename,
		ContentType:    contentType,
		Size:           size,
		Status:         MediaStatusPending,
		ScanStatus:     ScanStatusPending,
		RecordingMode:  RecordingModeRegular,
	}, nil
}

// IsPrivilege fails closed because OSS has no privilege-media lifecycle.
func (m *Media) IsPrivilege() bool { return false }

// SetEncrypted updates the media with encryption metadata and marks it as ready.
func (m *Media) SetEncrypted(storageKey string, wrappedDEK, wrappingNonce []byte, algo string, chunkSize, chunkCount int) {
	m.StorageKey = storageKey
	m.WrappedDEK = wrappedDEK
	m.WrappingNonce = wrappingNonce
	m.EncryptionAlgo = algo
	m.ChunkSize = chunkSize
	m.ChunkCount = chunkCount
	m.Status = MediaStatusReady
	m.UpdatedAt = time.Now()
}

// Clone returns a deep copy of the media.
func (m *Media) Clone() *Media {
	c := &Media{
		ID:                    m.ID,
		OrganizationID:        m.OrganizationID,
		CreatedBy:             m.CreatedBy,
		Filename:              m.Filename,
		Title:                 m.Title,
		Description:           m.Description,
		ContentType:           m.ContentType,
		Size:                  m.Size,
		Duration:              m.Duration,
		Status:                m.Status,
		StorageKey:            m.StorageKey,
		EncryptionAlgo:        m.EncryptionAlgo,
		ChunkSize:             m.ChunkSize,
		ChunkCount:            m.ChunkCount,
		FileHash:              m.FileHash,
		AudioDeletedAt:        m.AudioDeletedAt,
		ScanStatus:            m.ScanStatus,
		RecordingMode:         RecordingModeRegular,
		LatestTranscriptionID: m.LatestTranscriptionID,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}

	// Deep-copy byte slices to prevent shared mutation.
	if m.WrappedDEK != nil {
		c.WrappedDEK = make([]byte, len(m.WrappedDEK))
		copy(c.WrappedDEK, m.WrappedDEK)
	}
	if m.WrappingNonce != nil {
		c.WrappingNonce = make([]byte, len(m.WrappingNonce))
		copy(c.WrappingNonce, m.WrappingNonce)
	}
	if m.AudioMetadata != nil {
		c.AudioMetadata = make([]byte, len(m.AudioMetadata))
		copy(c.AudioMetadata, m.AudioMetadata)
	}
	if m.AudioDeletedAt != nil {
		t := *m.AudioDeletedAt
		c.AudioDeletedAt = &t
	}

	return c
}

// ValidateTitle trims whitespace and checks max length.
// Empty string is allowed (clears the title).
func ValidateTitle(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxTitleLength {
		return "", fmt.Errorf("title exceeds %d characters: %w", MaxTitleLength, ErrInvalidInput)
	}
	return s, nil
}

// ValidateDescription trims whitespace and checks max length.
// Empty string is allowed (clears the description).
func ValidateDescription(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxDescriptionLength {
		return "", fmt.Errorf("description exceeds %d characters: %w", MaxDescriptionLength, ErrInvalidInput)
	}
	return s, nil
}

// Trust level constants for audio forensic analysis.
const (
	TrustHigh    = "high"
	TrustMedium  = "medium"
	TrustLow     = "low"
	TrustUnknown = "unknown"
)

// AnalysisDisclaimer is shown alongside forensic analysis results.
const AnalysisDisclaimer = "This analysis checks file metadata and structure. It does not perform deep audio forensic analysis such as spectral examination or copy-move detection."

// AudioAnalysis holds forensic metadata extracted from an audio file.
// FormatTags and StreamTags are stored encrypted for audit purposes
// but are NOT exposed in API responses (see AudioAnalysisResponse in handler).
type AudioAnalysis struct {
	RecordingDate   string            `json:"recording_date,omitempty"`
	RecordingSource string            `json:"recording_source,omitempty"`
	Encoder         string            `json:"encoder,omitempty"`
	TrustLevel      string            `json:"trust_level"`
	Findings        []Finding         `json:"findings"`
	FormatName      string            `json:"format_name,omitempty"`
	CodecName       string            `json:"codec_name,omitempty"`
	SampleRate      int               `json:"sample_rate,omitempty"`
	Channels        int               `json:"channels,omitempty"`
	Bitrate         int64             `json:"bitrate,omitempty"`
	FormatTags      map[string]string `json:"format_tags,omitempty"`
	StreamTags      map[string]string `json:"stream_tags,omitempty"`
	Disclaimer      string            `json:"disclaimer"`
}

// Finding severity constants.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityLow    = "low"
	SeverityInfo   = "info"
)

// Finding status constants.
const (
	FindingPass = "pass"
	FindingFail = "fail"
	FindingWarn = "warn"
	FindingSkip = "skip"
)

// Finding represents a single integrity check result.
type Finding struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Summary  string `json:"summary"`
}
