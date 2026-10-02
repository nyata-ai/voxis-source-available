package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool timeout budgets. The default is for cheap list/get tools that already
// limit returned rows; the heavy budget covers tools that scan or aggregate.
const (
	defaultToolTimeout = 15 * time.Second
	heavyToolTimeout   = 30 * time.Second
)

// withTimeout wraps a tool handler with a request-context timeout. It returns
// a tool-level "timed out" error result when the deadline trips. The inner
// handler must observe ctx for the wrapper to take effect — wrapping is
// not preemption.
func withTimeout[In, Out any](tool string, d time.Duration, inner mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()

		res, out, err := inner(ctx, req, in)

		// Classify as timeout only when the inner handler signaled failure
		// AND the deadline tripped. A successful return (err==nil, res!=nil)
		// is preserved even if the deadline races past the function exit:
		// the work is already done.
		if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			recordOutcome(ctx, "timeout")
			var zero Out
			return errorResult(fmt.Errorf("%s: timed out", tool)), zero, nil
		}
		// If the handler returned no result and no error but the deadline did
		// fire, treat that as a timeout (defensive — handlers should not
		// silently produce nil/nil).
		if err == nil && res == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			recordOutcome(ctx, "timeout")
			var zero Out
			return errorResult(fmt.Errorf("%s: timed out", tool)), zero, nil
		}
		return res, out, err
	}
}
