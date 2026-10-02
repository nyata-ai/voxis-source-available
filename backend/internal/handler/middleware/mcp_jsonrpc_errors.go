package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// MCPJSONRPCErrorEnvelope wraps the response writer so any HTTP error emitted
// by downstream middleware on /mcp gets re-encoded as a JSON-RPC 2.0 error
// envelope. Spec-compliant MCP clients always parse /mcp POST responses as
// JSON-RPC, so a plain Gin error body (e.g. {"error":"unauthorized"})
// surfaces as "invalid response" — wrapping in the envelope lets the client
// surface the actual reason and follow the WWW-Authenticate hint.
//
// Behavior:
//   - status < 400: passes the buffered body through unchanged
//   - status >= 400: replaces the body with a JSON-RPC error envelope whose
//     message preserves the original Gin error text where possible
//   - all response headers (including WWW-Authenticate) are preserved
//
// JSON-RPC error codes used:
//
//	-32001  Unauthorized (HTTP 401)
//	-32002  Forbidden    (HTTP 403)
//	-32003  RateLimited  (HTTP 429)
//	-32004  Oversize     (HTTP 413)
//	-32600  BadRequest   (HTTP 400 — JSON-RPC InvalidRequest)
//	-32603  InternalError (HTTP 5xx — JSON-RPC InternalError)
//	-32099  generic transport failure for anything else 4xx
func MCPJSONRPCErrorEnvelope() gin.HandlerFunc {
	return func(c *gin.Context) {
		bw := &mcpJSONRPCWriter{ResponseWriter: c.Writer}
		c.Writer = bw

		// Catch panics inside this scope so we can emit a JSON-RPC envelope
		// instead of leaving the response empty. gin.Recovery (typically
		// mounted OUTER to this middleware) would normally turn a panic into
		// a plain text 500 — by intercepting here we keep the spec-compliant
		// envelope shape. The panic is logged via slog before being
		// translated; we deliberately do not re-panic, since translating to a
		// 500 envelope is the user-visible recovery action.
		var panicVal any
		func() {
			defer func() {
				panicVal = recover()
			}()
			c.Next()
		}()

		// Status precedence: panic → 500; otherwise explicit WriteHeader →
		// gin's stored status (set by c.Status / c.AbortWithStatus even when
		// no body is written) → 200.
		status := bw.status
		if panicVal != nil {
			slog.Default().Error("mcp handler panic",
				"panic", panicVal,
				"request_id", c.GetString("request_id"),
				"path", c.Request.URL.Path,
			)
			status = http.StatusInternalServerError
		}
		if status == 0 {
			status = bw.ResponseWriter.Status()
		}
		if status == 0 {
			status = http.StatusOK
		}
		bw.status = status

		if status < 400 {
			bw.flushOriginal()
			return
		}

		envelope := encodeJSONRPCError(status, bw.buf.Bytes())
		bw.replaceWith(envelope)
	}
}

// mcpJSONRPCWriter buffers writes so the wrapper can rewrite the body once
// the chain has run. Headers are still written through to the underlying
// ResponseWriter; only the body is held back.
type mcpJSONRPCWriter struct {
	gin.ResponseWriter
	buf     bytes.Buffer
	status  int
	wrote   bool
	flushed bool
}

func (w *mcpJSONRPCWriter) WriteHeader(status int) {
	// Capture status locally so the wrapper knows what was set, then forward
	// to gin's underlying writer so its internal `status` field stays in sync.
	// gin's WriteHeader only updates state; it does NOT flush headers to the
	// http.ResponseWriter, so we keep control over body substitution.
	w.status = status
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *mcpJSONRPCWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.buf.Write(b)
}

func (w *mcpJSONRPCWriter) WriteString(s string) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.buf.WriteString(s)
}

func (w *mcpJSONRPCWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *mcpJSONRPCWriter) Size() int {
	return w.buf.Len()
}

func (w *mcpJSONRPCWriter) Written() bool {
	return w.flushed
}

// Flush is intentionally a no-op while the wrapper is buffering. If we let it
// fall through to gin's responseWriter, gin would call WriteHeaderNow on the
// underlying writer and commit headers with the in-flight status — our defer
// would then be unable to send the JSON-RPC envelope as the response body.
// The MCP SDK runs in stateless JSON-response mode and does not stream;
// blocking Flush keeps that invariant from being broken by future middleware
// inserts. The full response is flushed once by our defer.
//
// If a future change wires SSE through this chain, the Flush no-op will
// silently buffer events — drop this middleware from streaming routes, or
// teach the wrapper to bypass buffering when Content-Type is text/event-stream.
func (w *mcpJSONRPCWriter) Flush() {
	// no-op by design; see comment above
}

func (w *mcpJSONRPCWriter) flushOriginal() {
	if w.flushed {
		return
	}
	w.flushed = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.Header().Set("Content-Length", strconv.Itoa(w.buf.Len()))
	w.ResponseWriter.WriteHeader(w.status)
	if _, err := w.ResponseWriter.Write(w.buf.Bytes()); err != nil {
		// Client likely disconnected. Nothing actionable for the middleware;
		// the request/audit pipeline already logged the underlying handler.
		_ = err
	}
}

func (w *mcpJSONRPCWriter) replaceWith(body []byte) {
	if w.flushed {
		return
	}
	w.flushed = true
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.ResponseWriter.WriteHeader(w.status)
	if _, err := w.ResponseWriter.Write(body); err != nil {
		_ = err
	}
}

// ginErrorBody captures the small subset of fields Gin's AbortWithStatusJSON
// emits — used to recover a human message for the JSON-RPC envelope without
// leaking implementation details.
type ginErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func encodeJSONRPCError(status int, body []byte) []byte {
	code := jsonRPCCodeForStatus(status)
	msg := jsonRPCMessageForStatus(status)
	if len(body) > 0 {
		var parsed ginErrorBody
		if err := json.Unmarshal(body, &parsed); err == nil {
			if parsed.Message != "" {
				msg = parsed.Message
			} else if parsed.Error != "" {
				msg = parsed.Error
			}
		}
	}
	envelope := map[string]any{
		"jsonrpc": "2.0",
		"id":      nil,
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	}
	out, err := json.Marshal(envelope)
	if err != nil {
		// json.Marshal on a constant-shape map cannot fail in practice; fall
		// back to a static envelope so the client still sees JSON-RPC, not
		// a corrupted body, if the unthinkable happens.
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return out
}

func jsonRPCCodeForStatus(status int) int {
	switch status {
	case http.StatusUnauthorized:
		return -32001
	case http.StatusForbidden:
		return -32002
	case http.StatusTooManyRequests:
		return -32003
	case http.StatusRequestEntityTooLarge:
		return -32004
	case http.StatusBadRequest:
		return -32600
	}
	if status >= 500 {
		return -32603
	}
	return -32099
}

func jsonRPCMessageForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusTooManyRequests:
		return "rate limited"
	case http.StatusRequestEntityTooLarge:
		return "payload too large"
	case http.StatusBadRequest:
		return "bad request"
	}
	if status >= 500 {
		return "internal error"
	}
	return http.StatusText(status)
}
