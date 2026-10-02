package service

import (
	"fmt"
	"math"

	"github.com/voxis/backend/internal/domain"
)

// scratchReserveBytes stays free on a scratch filesystem beyond a job's own
// estimate, so one media job cannot fill the disk other jobs and the database
// spool share.
const scratchReserveBytes int64 = 512 << 20

// EnsureScratchSpace fails with domain.ErrInsufficientScratchSpace unless dir's
// filesystem has requiredBytes plus the fixed reserve available.
func EnsureScratchSpace(dir string, requiredBytes int64) error {
	if dir == "" || requiredBytes < 0 {
		return fmt.Errorf("scratch space check needs a directory and a non-negative size: %w", domain.ErrInvalidInput)
	}
	free, err := freeSpaceBytes(dir)
	if err != nil {
		return fmt.Errorf("check free scratch space: %w", err)
	}
	needed := requiredBytes + scratchReserveBytes
	if free > math.MaxInt64 || int64(free) >= needed {
		return nil
	}
	return fmt.Errorf("scratch filesystem has %d MiB free, job needs %d MiB: %w",
		free>>20, needed>>20, domain.ErrInsufficientScratchSpace)
}
