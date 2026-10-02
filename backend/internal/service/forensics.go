package service

import (
	"math"
	"strings"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// durationMismatchThreshold is the maximum acceptable difference (in seconds)
// between format-level and stream-level duration before flagging a warning.
const durationMismatchThreshold = 0.5

// formatExtensionMap maps file extensions to expected ffprobe format_name patterns.
var formatExtensionMap = map[string][]string{
	".mp3":  {"mp3"},
	".wav":  {"wav"},
	".ogg":  {"ogg"},
	".flac": {"flac"},
	".m4a":  {"mov,mp4,m4a", "mp4"},
	".mp4":  {"mov,mp4,m4a", "mp4"},
	".webm": {"matroska,webm", "webm"},
	".aac":  {"aac"},
	".opus": {"ogg", "matroska,webm"},
	".wma":  {"asf"},
}

// codecContainerMismatches lists encoder signatures that indicate re-encoding
// when found in specific container formats.
var codecContainerMismatches = map[string][]string{
	"wav": {"LAME", "lame", "libmp3lame"},
	"ogg": {"LAME", "lame", "libmp3lame"},
}

// BuildAnalysis runs integrity checks on probe results and returns an AudioAnalysis.
func BuildAnalysis(probe *port.AudioProbeResult, fileExt string) *domain.AudioAnalysis {
	if probe == nil {
		return &domain.AudioAnalysis{
			TrustLevel: domain.TrustUnknown,
			Findings:   []domain.Finding{{Check: "probe", Severity: domain.SeverityHigh, Status: domain.FindingSkip, Summary: "Could not analyze audio file"}},
			Disclaimer: domain.AnalysisDisclaimer,
		}
	}

	analysis := &domain.AudioAnalysis{
		FormatName: probe.FormatName,
		CodecName:  probe.Codec,
		SampleRate: probe.SampleRate,
		Channels:   probe.Channels,
		Bitrate:    probe.Bitrate,
		FormatTags: probe.FormatTags,
		StreamTags: probe.StreamTags,
		Disclaimer: domain.AnalysisDisclaimer,
	}

	// Extract recording metadata from tags
	analysis.RecordingDate = extractRecordingDate(probe.FormatTags, probe.StreamTags)
	analysis.RecordingSource = extractRecordingSource(probe.FormatTags, probe.StreamTags)
	analysis.Encoder = extractEncoder(probe.FormatTags, probe.StreamTags)

	// Run all 5 integrity checks
	findings := []domain.Finding{
		checkFormatConsistency(probe.FormatName, fileExt),
		checkCodecContainerPlausibility(probe.FormatName, probe.FormatTags, probe.StreamTags),
		checkDurationConsistency(probe.FormatDuration, probe.StreamDuration),
		checkReEncodingIndicators(probe.FormatTags, probe.StreamTags),
		checkMetadataCompleteness(probe.FormatName, probe.FormatTags, probe.StreamTags),
	}

	analysis.Findings = findings
	analysis.TrustLevel = calculateTrustLevel(findings)

	return analysis
}

func extractRecordingDate(formatTags, streamTags map[string]string) string {
	return firstNonEmpty(
		formatTags["com.apple.quicktime.creationdate"],
		formatTags["creation_time"],
		streamTags["creation_time"],
		formatTags["date"],
		formatTags["time"],
	)
}

func extractRecordingSource(formatTags, streamTags map[string]string) string {
	return firstNonEmpty(
		formatTags["com.apple.quicktime.model"],
		formatTags["originator"],
		formatTags["com.android.version"],
		streamTags["handler_name"],
	)
}

func extractEncoder(formatTags, streamTags map[string]string) string {
	return firstNonEmpty(formatTags["encoder"], streamTags["encoder"])
}

func checkFormatConsistency(formatName, fileExt string) domain.Finding {
	f := domain.Finding{Check: "format_consistency", Severity: domain.SeverityHigh}
	ext := strings.ToLower(fileExt)
	if ext == "" {
		f.Status = domain.FindingSkip
		f.Summary = "No file extension, cannot verify format"
		return f
	}
	expected, ok := formatExtensionMap[ext]
	if !ok {
		f.Status = domain.FindingSkip
		f.Summary = "Unknown file extension, cannot verify format"
		return f
	}
	for _, e := range expected {
		if strings.Contains(formatName, e) {
			f.Status = domain.FindingPass
			f.Summary = "File format matches extension"
			return f
		}
	}
	f.Status = domain.FindingFail
	f.Summary = "File format (" + formatName + ") does not match extension (" + ext + ")"
	return f
}

// checkCodecContainerPlausibility detects encoder/container mismatches
// (e.g., LAME encoder in WAV container = re-encoded MP3 in WAV wrapper).
func checkCodecContainerPlausibility(formatName string, formatTags, streamTags map[string]string) domain.Finding {
	f := domain.Finding{Check: "codec_container_plausibility", Severity: domain.SeverityMedium}
	encoder := firstNonEmpty(formatTags["encoder"], streamTags["encoder"])
	if encoder == "" {
		f.Status = domain.FindingSkip
		f.Summary = "No encoder tag to check against container"
		return f
	}

	for container, badEncoders := range codecContainerMismatches {
		if !strings.Contains(formatName, container) {
			continue
		}
		for _, bad := range badEncoders {
			if strings.Contains(encoder, bad) {
				f.Status = domain.FindingWarn
				f.Summary = "Encoder (" + encoder + ") is unusual for " + container + " container, suggests re-encoding"
				return f
			}
		}
	}

	f.Status = domain.FindingPass
	f.Summary = "Encoder is consistent with container format"
	return f
}

func checkDurationConsistency(formatDur, streamDur float64) domain.Finding {
	f := domain.Finding{Check: "duration_consistency", Severity: domain.SeverityMedium}
	if formatDur <= 0 || streamDur <= 0 {
		f.Status = domain.FindingSkip
		f.Summary = "Duration data unavailable for comparison"
		return f
	}
	diff := math.Abs(formatDur - streamDur)
	if diff > durationMismatchThreshold {
		f.Status = domain.FindingWarn
		f.Summary = "Container and stream durations differ significantly"
		return f
	}
	f.Status = domain.FindingPass
	f.Summary = "Container and stream durations are consistent"
	return f
}

func checkReEncodingIndicators(formatTags, streamTags map[string]string) domain.Finding {
	f := domain.Finding{Check: "re_encoding_indicators", Severity: domain.SeverityLow}
	encoder := firstNonEmpty(formatTags["encoder"], streamTags["encoder"])
	if strings.Contains(encoder, "Lavf") || strings.Contains(encoder, "Lavc") {
		f.Status = domain.FindingWarn
		f.Summary = "File was processed with FFmpeg (" + encoder + ")"
		return f
	}
	f.Status = domain.FindingPass
	f.Summary = "No re-encoding indicators detected"
	return f
}

func checkMetadataCompleteness(formatName string, formatTags, streamTags map[string]string) domain.Finding {
	f := domain.Finding{Check: "metadata_completeness", Severity: domain.SeverityLow}
	// Formats that normally carry creation_time
	expectsCreationTime := strings.Contains(formatName, "mp4") ||
		strings.Contains(formatName, "m4a") ||
		strings.Contains(formatName, "mov")
	if expectsCreationTime {
		ct := firstNonEmpty(formatTags["creation_time"], streamTags["creation_time"])
		if ct == "" {
			f.Status = domain.FindingWarn
			f.Summary = "Expected creation timestamp missing from MP4/M4A container"
			return f
		}
	}
	f.Status = domain.FindingPass
	f.Summary = "Metadata fields are present as expected"
	return f
}

func calculateTrustLevel(findings []domain.Finding) string {
	highCount := 0
	mediumCount := 0
	for _, f := range findings {
		if f.Status != domain.FindingFail && f.Status != domain.FindingWarn {
			continue
		}
		switch f.Severity {
		case domain.SeverityHigh:
			highCount++
		case domain.SeverityMedium:
			mediumCount++
		}
	}
	if highCount > 0 || mediumCount >= 2 {
		return domain.TrustLow
	}
	if mediumCount == 1 {
		return domain.TrustMedium
	}
	// Check for any low-severity warnings
	for _, f := range findings {
		if f.Status == domain.FindingWarn && f.Severity == domain.SeverityLow {
			return domain.TrustMedium
		}
	}
	return domain.TrustHigh
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
