package system

import (
	"context"
	"fmt"
	"net/http"
)

// HTTPProbe returns a dependency probe that GETs url and fails on error or non-2xx.
// The request honors the caller's context, so the per-check timeout applies.
func HTTPProbe(url string) func(context.Context) error {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close() //nolint:errcheck // best-effort close on HTTP response
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}
}
