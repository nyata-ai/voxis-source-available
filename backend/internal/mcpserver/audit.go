package mcpserver

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/handler/middleware"
)

// outcomeKey is the context key under which auditCall installs an outcome
// label pointer that helpers (toolError, withTimeout, scope checks) write to.
// The audit wrapper reads it after the inner handler returns.
type outcomeKey struct{}

// recordOutcome stores the outcome label for the current tool call. Helpers
// closer to the failure (validation, scope, deadline) write the label so the
// audit wrapper can read a stable enum without parsing error messages.
//
// Last write wins; helpers are written so only the deepest classification
// reaches the channel.
func recordOutcome(ctx context.Context, label string) {
	if p, ok := ctx.Value(outcomeKey{}).(*string); ok && p != nil {
		*p = label
	}
}

// wrapTool composes audit + timeout in registration order: audit reads the
// outcome label that helpers (and withTimeout) write, so audit must wrap
// withTimeout from the outside.
func wrapTool[In, Out any](
	name string,
	budget time.Duration,
	deps Dependencies,
	inner mcp.ToolHandlerFor[In, Out],
) mcp.ToolHandlerFor[In, Out] {
	return auditCall[In, Out](name, deps, withTimeout[In, Out](name, budget, inner))
}

// toolRegistry records the name of every tool added to an MCP server, so the
// capability resources describe exactly the registered set.
type toolRegistry struct {
	server *mcp.Server
	names  []string
}

func registerTool[In, Out any](
	registry *toolRegistry,
	tool *mcp.Tool,
	handler mcp.ToolHandlerFor[In, Out],
) {
	annotateTool(tool)
	mcp.AddTool(registry.server, tool, handler)
	registry.names = append(registry.names, tool.Name)
}

func annotateTool(tool *mcp.Tool) {
	if tool == nil {
		return
	}
	spec := catalogTool(tool.Name)
	tool.Title = spec.Title
	tool.Description = spec.Description
	tool.Annotations = spec.Annotations
	tool.Annotations.Title = spec.Title
}

// auditCall wraps an MCP tool handler with a single audit log line per call.
// It installs the outcome side-channel, lets helpers classify the result, and
// emits a stable structured log line. The wrapper does not log tool arguments.
//
// Wrap inside withTimeout: registration looks like
//
//	auditCall(name, deps, withTimeout(name, d, handler))
func auditCall[In, Out any](
	name string,
	deps Dependencies,
	inner mcp.ToolHandlerFor[In, Out],
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var label string
		ctx = context.WithValue(ctx, outcomeKey{}, &label)

		start := time.Now()
		result, out, err := inner(ctx, req, in)
		latency := time.Since(start)

		outcome := label
		if outcome == "" {
			switch {
			case err != nil:
				outcome = "internal_error"
			case result != nil && result.IsError:
				outcome = "tool_error"
			default:
				outcome = "ok"
			}
		}

		if deps.Logger != nil {
			deps.Logger.Info("mcp tool call",
				"request_id", contextString(ctx, middleware.CtxKeyRequestID),
				"tool", name,
				"auth_method", contextString(ctx, middleware.CtxKeyAuthMethod),
				"org_id", contextString(ctx, middleware.CtxKeyOrgID),
				"api_key_id", contextString(ctx, middleware.CtxKeyAPIKeyID),
				"outcome", outcome,
				"latency_ms", latency.Milliseconds(),
			)
		}

		return result, out, err
	}
}
