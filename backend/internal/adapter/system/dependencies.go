package system

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/voxis/backend/internal/port"
)

// DependencyCheck is one named health probe. Exported so main.go can build the
// probe list. Probe SHOULD honor ctx, but checkDependencies enforces the
// per-check deadline even if it does not (e.g. ClamAV's Available ignores ctx).
type DependencyCheck struct {
	Name  string
	Probe func(ctx context.Context) error
}

// checkDependencies runs all probes concurrently and returns results in input
// order. Two bounds apply (matching the design): an overall budget across all
// checks, and a per-check timeout. Each probe runs in its own goroutine; we
// select on probe-done vs the per-check context, so a probe that ignores its
// context cannot block past min(perCheck, remaining overall budget). The
// orphaned goroutine writes to a buffered channel and exits harmlessly.
// Bounded by len(checks) (Power-of-10 rule 2).
func checkDependencies(ctx context.Context, checks []DependencyCheck, now func() time.Time, perCheck, overall time.Duration) []port.SystemDependency {
	octx, cancelAll := context.WithTimeout(ctx, overall)
	defer cancelAll()

	results := make([]port.SystemDependency, len(checks))
	var wg sync.WaitGroup
	wg.Add(len(checks))
	for i := range checks {
		go func(idx int) {
			defer wg.Done()
			c := checks[idx]
			cctx, cancel := context.WithTimeout(octx, perCheck)
			defer cancel()
			start := now()

			done := make(chan error, 1) // buffered: orphaned probe never blocks
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("probe panicked: %v", r)
					}
				}()
				done <- c.Probe(cctx)
			}()

			var err error
			select {
			case err = <-done:
			case <-cctx.Done():
				err = cctx.Err() // per-check timeout or overall budget exceeded
			}

			latency := now().Sub(start).Milliseconds()
			d := port.SystemDependency{Name: c.Name, LatencyMS: latency, Status: "up"}
			if err != nil {
				d.Status = "down"
				d.Reason = err.Error()
			}
			results[idx] = d
		}(i)
	}
	wg.Wait()
	return results
}
