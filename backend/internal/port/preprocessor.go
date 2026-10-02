// backend/internal/port/preprocessor.go
package port

import "context"

// PreprocessResult holds metadata about what preprocessing was applied.
type PreprocessResult struct {
	// OutputPath is the path to the preprocessed file.
	OutputPath string

	// Applied is true if filters were actually applied (false = passthrough).
	Applied bool

	// PreprocessorName identifies which preprocessor ran (e.g., "ffmpeg", "deepfilter").
	PreprocessorName string

	// DurationSecs is the audio duration after processing (0 if unknown).
	DurationSecs float64
}

// AudioPreprocessor preprocesses audio files before transcription.
// Implementations must be safe for concurrent use.
type AudioPreprocessor interface {
	// Preprocess applies audio processing to inputPath, writing to outputPath.
	// The outputPath will always be WAV format (16kHz, mono, s16).
	// Returns metadata about what was applied.
	Preprocess(ctx context.Context, inputPath, outputPath string) (*PreprocessResult, error)

	// Available reports whether this preprocessor's binary dependencies are installed.
	Available() bool

	// Name returns a human-readable identifier (e.g., "ffmpeg", "deepfilter").
	Name() string
}
