package domain

import (
	"fmt"
	"time"
)

// DefaultMaxMediaDuration is the longest audio accepted when
// MEDIA_MAX_DURATION is unset. A 1 GB low-bitrate file can hold hundreds of
// hours, which would expand to tens of gigabytes of PCM during processing. It
// matches the default RECORDING_MAX_DURATION_HOURS so a finished recording is
// never refused.
const DefaultMaxMediaDuration = 8 * time.Hour

// MediaDurationGrace is added to the limit for stitched recordings and for the
// ffmpeg output cap. The recording limit is enforced on wall-clock time when a
// chunk arrives, so the final chunk and container timing can carry a stitched
// recording slightly past it; accepted media must never be truncated.
const MediaDurationGrace = 15 * time.Minute

// CheckMediaDuration returns ErrMediaTooLong when durationSeconds exceeds
// limit. A non-positive duration (unknown) or limit (no limit) passes.
func CheckMediaDuration(durationSeconds float64, limit time.Duration) error {
	if limit <= 0 || durationSeconds <= 0 {
		return nil
	}
	if durationSeconds > limit.Seconds() {
		return fmt.Errorf("audio is %s long; the maximum is %s: %w",
			time.Duration(durationSeconds*float64(time.Second)).Round(time.Second), limit, ErrMediaTooLong)
	}
	return nil
}
