package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func cleanupStaleTempFiles(logger *slog.Logger) {
	threshold := time.Now().Add(-time.Hour)
	for _, root := range []string{os.TempDir(), "/dev/shm"} {
		for _, pattern := range []string{"voxis-probe-*.tmp", "voxis-stitch-*", "voxis-upload-*.tmp", "voxis-transcribe-source-*", "voxis-transcribe-chunks-*", "voxis-stripped-*", "voxis-preprocess-*", "voxis-sm-spool-*"} {
			removeStaleTempMatches(logger, filepath.Join(root, pattern), threshold)
		}
	}
}

func removeStaleTempMatches(logger *slog.Logger, pattern string, threshold time.Time) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		logger.Warn("list stale temporary files", "pattern", pattern, "error", err)
		return
	}
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().Before(threshold) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			logger.Warn("remove stale temporary file", "path", path, "error", err)
		}
	}
}
