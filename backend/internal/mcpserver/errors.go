package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/handler/middleware"
)

// toolError converts a service-layer error into a sanitized MCP CallToolResult.
//
// Rules:
//   - Domain sentinels map to short, safe strings ({op}: not found, etc.).
//   - ErrInvalidInput surfaces the validation message minus the sentinel prefix.
//   - All other errors return "{op}: internal error" — the raw error is logged
//     with full correlation fields so an operator can reconstruct the failure.
//
// Caller should `return toolError(...), nil, nil` to propagate as a tool-level
// error result (the MCP SDK passes IsError=true bodies back to the model).
func toolError(ctx context.Context, logger *slog.Logger, tool, op string, err error) *mcp.CallToolResult {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: not found", op))
	case errors.Is(err, domain.ErrForbidden):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: access denied", op))
	case errors.Is(err, domain.ErrConflict):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: conflict", op))
	case errors.Is(err, domain.ErrCreditsExpired):
		// Checked before the shortfall case: expiry demands a different user
		// action (buy more to restore access, not top up a shortfall). Mirrors
		// the REST handlers (transcription.go, url_transcription.go) so the
		// same user sees consistent wording across surfaces.
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: your credits have expired — purchase more to restore access", op))
	case errors.Is(err, domain.ErrInsufficientCredits):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: your organization is out of credits — top up to start new transcriptions", op))
	case errors.Is(err, domain.ErrInvalidInput):
		recordOutcome(ctx, "validation_error")
		return errorResult(fmt.Errorf("%s: %s", op, stripInvalidInputSentinel(err)))
	case errors.Is(err, domain.ErrRateLimited):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: rate limited", op))
	case errors.Is(err, domain.ErrModelBusy):
		recordOutcome(ctx, "tool_error")
		return errorResult(fmt.Errorf("%s: model_busy: the local model is busy, try again shortly", op))
	}

	recordOutcome(ctx, "internal_error")

	if logger != nil {
		logger.Error("mcp tool internal error",
			"request_id", contextString(ctx, middleware.CtxKeyRequestID),
			"tool", tool,
			"op", op,
			"org_id", contextString(ctx, middleware.CtxKeyOrgID),
			"api_key_id", contextString(ctx, middleware.CtxKeyAPIKeyID),
			"auth_method", contextString(ctx, middleware.CtxKeyAuthMethod),
			"error", err.Error(),
		)
	}
	return errorResult(fmt.Errorf("%s: internal error", op))
}

// validationError labels the outcome and returns a validation-error result.
// Use this for handler-local validation failures that do not wrap a domain
// sentinel — it keeps the audit outcome enum stable without forcing every
// caller to remember the recordOutcome call.
func validationError(ctx context.Context, err error) *mcp.CallToolResult {
	recordOutcome(ctx, "validation_error")
	return errorResult(err)
}

// stripInvalidInputSentinel returns the user-facing portion of an
// ErrInvalidInput-wrapped error. The convention in this codebase is
// `fmt.Errorf("foo: %w", ErrInvalidInput)`, which yields "foo: invalid input".
// We strip the trailing sentinel string so the user sees "foo".
func stripInvalidInputSentinel(err error) string {
	msg := err.Error()
	const suffix = ": " + "invalid input"
	if strings.HasSuffix(msg, suffix) {
		return strings.TrimSuffix(msg, suffix)
	}
	if msg == "invalid input" {
		return "invalid input"
	}
	return msg
}

// contextString reads a string from context.Value, returning "" if absent.
func contextString(ctx context.Context, key any) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(key)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
