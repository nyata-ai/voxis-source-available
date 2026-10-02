package service

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voxis/backend/internal/domain"
)

func TestCreateStitchTmpDirIsPrivateAndUnpredictable(t *testing.T) {
	first, err := createStitchTmpDir(1 << 20)
	if err != nil {
		t.Fatalf("createStitchTmpDir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(first) })
	second, err := createStitchTmpDir(1 << 20)
	if err != nil {
		t.Fatalf("createStitchTmpDir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(second) })

	if first == second {
		t.Fatalf("two stitch directories share the path %q", first)
	}
	if !strings.HasPrefix(filepath.Base(first), stitchDirPrefix) {
		t.Fatalf("stitch directory %q lacks the %q prefix the stale sweep matches", first, stitchDirPrefix)
	}
	info, err := os.Stat(first)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("stitch directory mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestEnsureScratchSpace(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureScratchSpace(dir, 1); err != nil {
		t.Fatalf("EnsureScratchSpace(1 byte) error = %v", err)
	}
	if err := EnsureScratchSpace(dir, math.MaxInt64/2); !errors.Is(err, domain.ErrInsufficientScratchSpace) {
		t.Fatalf("EnsureScratchSpace(huge) error = %v, want ErrInsufficientScratchSpace", err)
	}
	if err := EnsureScratchSpace(filepath.Join(dir, "missing"), 1); err == nil {
		t.Fatal("EnsureScratchSpace(missing dir) error = nil")
	}
}
