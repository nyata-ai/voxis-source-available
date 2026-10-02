package domain

// RecordingMode is kept only so a standard recording request can reject a
// legacy special-mode value clearly. OSS persists and accepts regular mode.
type RecordingMode string

// Recording modes include the legacy value so the OSS boundary can reject it.
const (
	RecordingModeRegular   RecordingMode = "regular"
	RecordingModePrivilege RecordingMode = "privilege"
)

// IsValid reports whether a request supplied a known recording mode.
func (m RecordingMode) IsValid() bool {
	return m == RecordingModeRegular || m == RecordingModePrivilege
}
