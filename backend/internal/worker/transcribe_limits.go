package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voxis/backend/internal/adapter/ffmpeg"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/service"
)

const (
	// pcm16kBytesPerSecond is 16 kHz mono 16-bit PCM, the Speechmatics chunk format.
	pcm16kBytesPerSecond = 16000 * 2
	// pcm48kBytesPerSecond is 48 kHz mono 16-bit PCM, the DeepFilterNet format.
	pcm48kBytesPerSecond = 48000 * 2
	// maxUploadExtLen bounds the extension kept in the provider filename.
	maxUploadExtLen = 5
)

// ffmpegOptions are the sandbox and duration settings every worker-side ffmpeg
// run shares.
func (w *TranscribeWorker) ffmpegOptions() []ffmpeg.Option {
	return []ffmpeg.Option{
		ffmpeg.WithSandboxRequired(w.sandboxRequired),
		ffmpeg.WithMaxOutputDuration(w.maxMediaDuration),
	}
}

// ensureScratchSpace fails the attempt before any decryption, conversion, or
// split when the scratch filesystem cannot hold the job's working files. The
// error is retryable; the last attempt marks the transcription failed.
func (w *TranscribeWorker) ensureScratchSpace(trans *domain.Transcription, media *domain.Media) error {
	required := estimateTranscribeScratchBytes(media, trans.EnhanceAudio, w.maxMediaDuration)
	if err := service.EnsureScratchSpace(os.TempDir(), required); err != nil {
		return fmt.Errorf("transcription scratch space: %w", err)
	}
	return nil
}

// estimateTranscribeScratchBytes bounds the worst case the worker writes at
// once: the decrypted source plus one rewritten copy, and the PCM produced by
// the 16 kHz split (and, with enhancement, the 48 kHz normalized and enhanced
// files). An unknown duration is treated as the maximum allowed.
func estimateTranscribeScratchBytes(media *domain.Media, enhance bool, maxDuration time.Duration) int64 {
	seconds := media.Duration
	if seconds <= 0 {
		seconds = maxDuration.Seconds()
	}
	perSecond := float64(pcm16kBytesPerSecond)
	if enhance {
		perSecond += 2 * pcm48kBytesPerSecond
	}
	return 2*max(media.Size, 0) + int64(seconds*perSecond)
}

// providerUploadName is the filename sent to the transcription provider. The
// user's filename can itself be personal data, so only a short alphanumeric
// extension survives, which the provider needs to recognize the container.
func providerUploadName(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) < 2 || len(ext) > maxUploadExtLen+1 {
		return "audio"
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return "audio"
		}
	}
	return "audio" + ext
}
