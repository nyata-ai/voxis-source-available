package ffmpeg

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"time"
)

// mimeToFFmpegExt maps audio MIME types to file extensions that ffmpeg
// understands for output format detection. Using the correct extension
// is critical — ffmpeg cannot infer format from .tmp.
var mimeToFFmpegExt = map[string]string{
	"audio/mpeg":  ".mp3",
	"audio/wav":   ".wav",
	"audio/x-wav": ".wav",
	"audio/ogg":   ".ogg",
	"audio/flac":  ".flac",
	"audio/mp4":   ".m4a",
	"audio/x-m4a": ".m4a",
	"audio/webm":  ".webm",
	"audio/aac":   ".aac",
	"audio/opus":  ".ogg", // Opus in OGG container
}

// StripMetadata removes all metadata from an audio file using ffmpeg.
// It writes the cleaned file to a unique temp file and returns its path.
// The caller is responsible for removing the output file.
//
// contentType determines the output container format (e.g., "audio/webm" -> .webm).
// Using the correct extension is critical because ffmpeg infers output format
// from the file extension — a .tmp extension would cause format detection failure.
func StripMetadata(ctx context.Context, inputPath, contentType string, opts ...Option) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// Determine output extension from content type.
	mediaType, _, parseErr := mime.ParseMediaType(contentType)
	if parseErr != nil {
		mediaType = contentType
	}
	ext := mimeToFFmpegExt[mediaType]
	if ext == "" {
		ext = ".bin" // fallback — use -f flag below
	}

	// Use os.CreateTemp for a unique, unpredictable output path
	// (prevents symlink attacks and concurrent worker collisions).
	outFile, createErr := os.CreateTemp("", "voxis-stripped-*"+ext)
	if createErr != nil {
		return "", fmt.Errorf("create temp output file: %w", createErr)
	}
	outPath := outFile.Name()
	_ = outFile.Close() //nolint:errcheck // ffmpeg will write to this path
	work, err := os.MkdirTemp("", "voxis-strip-*")
	if err != nil {
		_ = os.Remove(outPath) //nolint:errcheck // best-effort cleanup after failure
		return "", err
	}
	defer func() { _ = os.RemoveAll(work) }() //nolint:errcheck // best-effort cleanup of our private directory
	privateOutput := filepath.Join(work, "output"+ext)

	args := []string{
		"-i", inputPath,
		"-map", "0:a", // keep audio streams only
		"-vn",                 // drop video streams (including attached pictures)
		"-sn",                 // drop subtitle streams
		"-dn",                 // drop data streams
		"-map_metadata", "-1", // strip all metadata
		"-c", "copy", // no re-encoding (fast, preserves audio quality)
		"-y", // overwrite the empty temp file
	}

	// If extension is the fallback .bin, explicitly set output format
	// based on input (ffmpeg will probe the input to determine format).
	if ext == ".bin" {
		args = append(args, "-f", "matroska") // safe fallback container
	}

	args = append(args, privateOutput)

	//nolint:gosec // binary name is a constant "ffmpeg"
	cmd, err := New(nil, opts...).command(ctx, args)
	if err != nil {
		_ = os.Remove(outPath) //nolint:errcheck // best-effort cleanup after failure
		return "", err
	}
	output := &limitedWriter{max: maxStderrBytes}
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	if err != nil {
		_ = os.Remove(outPath) //nolint:errcheck // clean up on failure
		return "", fmt.Errorf("ffmpeg strip metadata: %w (output: %s)", err, output.String())
	}
	if err := os.Rename(privateOutput, outPath); err != nil {
		_ = os.Remove(outPath) //nolint:errcheck // best-effort cleanup after failure
		return "", err
	}
	return outPath, nil
}
