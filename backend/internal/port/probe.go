package port

import "context"

// AudioProbeResult contains metadata extracted from an audio file.
type AudioProbeResult struct {
	Duration   float64 // seconds (e.g., 183.456)
	Codec      string  // codec name (e.g., "mp3", "aac", "opus")
	SampleRate int     // Hz (e.g., 44100)
	Channels   int     // 1=mono, 2=stereo
	Bitrate    int64   // bits per second
	// Forensic metadata
	FormatName     string            // e.g., "mov,mp4,m4a,3gp,3g2,mj2"
	FormatLongName string            // e.g., "QuickTime / MOV"
	ProbeScore     int               // 0-100 format detection confidence
	FormatTags     map[string]string // all format-level tags
	StreamTags     map[string]string // all audio stream tags
	FormatDuration float64           // container-level duration for consistency check
	StreamDuration float64           // stream-level duration
}

// AudioProber extracts metadata from audio files.
type AudioProber interface {
	// Probe extracts audio metadata from the file at the given path.
	// Returns domain.ErrInvalidInput if the file is not valid audio.
	Probe(ctx context.Context, filePath string) (*AudioProbeResult, error)

	// Available reports whether the probe binary is accessible.
	Available() bool
}
