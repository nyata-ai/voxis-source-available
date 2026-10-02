package port

import "context"

// AudioChunker splits audio files into sequential chunks.
type AudioChunker interface {
	// SplitByDuration splits inputPath into chunk files at most maxDurationSec each.
	// Returns output files in chronological order.
	// sourceContentType should be the original media MIME type (e.g., "audio/mpeg").
	SplitByDuration(ctx context.Context, inputPath, outputDir string, maxDurationSec int, sourceContentType string) ([]string, error)

	// SplitByDurationWAV splits inputPath into 16-bit 16 kHz mono WAV chunks of
	// at most maxDurationSec each, in chronological order. The source container
	// is irrelevant: every chunk is decoded and re-encoded to PCM, the input
	// format Speechmatics documents as optimal. Prefer SplitByDuration when the
	// provider accepts the source container, since this one always re-encodes.
	SplitByDurationWAV(ctx context.Context, inputPath, outputDir string, maxDurationSec int) ([]string, error)
}
