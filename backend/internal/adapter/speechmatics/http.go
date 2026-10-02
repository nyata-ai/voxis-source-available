package speechmatics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/voxis/backend/internal/domain"
)

// newRequest builds an authorized request against the configured base URL.
func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("speechmatics: create %s request: %w", method, err)
	}
	c.authorize(req)
	return req, nil
}

// authorize attaches the bearer credential. Container mode reaches a private
// endpoint that carries no account key, so it sends none.
func (c *Client) authorize(req *http.Request) {
	if c.cfg.Mode == ModeContainer || c.cfg.APIKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
}

// doShortGet issues a rate-limited GET with its own deadline.
func (c *Client) doShortGet(ctx context.Context, path string, timeout time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)

	if err := c.getLimiter.Wait(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("speechmatics: rate limiter: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodGet, path)
	if err != nil {
		cancel()
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("speechmatics: GET %s: %w", path, err)
	}
	// The deadline must outlive the header round trip so the body can be read;
	// it is released when the caller closes the body.
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnCloseBody releases a request's context when its body is closed.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// newMultipartJobRequest streams the staged file and the job config as one
// multipart body. io.Pipe keeps the file out of memory.
func newMultipartJobRequest(
	ctx context.Context, endpoint, audioPath string, meta uploadMeta, configJSON []byte,
) (*http.Request, error) {
	f, err := os.Open(audioPath) //nolint:gosec // parseSpoolToken proved the path is in this client's spool dir
	if err != nil {
		return nil, fmt.Errorf("speechmatics: open staged audio: %w", err)
	}

	// Speechmatics sniffs the container format from these two headers. The
	// staged file is named audio-<rand>.bin, so without the original name and
	// type every submission would look like an unknown ".bin" blob.
	filename := safeUploadFilename(meta.filename)
	if filename == "" {
		filename = filepath.Base(audioPath)
	}

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer f.Close() //nolint:errcheck // best-effort close of the staged file
		if wErr := writeJobParts(writer, f, filename, safeContentType(meta.contentType), configJSON); wErr != nil {
			_ = pw.CloseWithError(wErr) //nolint:errcheck // pipe close cannot fail
			return
		}
		if cErr := writer.Close(); cErr != nil {
			_ = pw.CloseWithError(cErr) //nolint:errcheck // pipe close cannot fail
			return
		}
		_ = pw.Close() //nolint:errcheck // pipe close cannot fail
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		_ = pr.CloseWithError(err) //nolint:errcheck // unblocks the writer goroutine
		return nil, fmt.Errorf("speechmatics: create submit request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

// writeJobParts writes the data_file and config parts in that order.
func writeJobParts(
	writer *multipart.Writer, audio io.Reader, filename, contentType string, configJSON []byte,
) error {
	header := make(textproto.MIMEHeader, 2)
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="data_file"; filename=%q`, filename))
	header.Set("Content-Type", contentType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("speechmatics: create data_file part: %w", err)
	}
	if _, err := io.Copy(part, audio); err != nil {
		return fmt.Errorf("speechmatics: stream staged audio: %w", err)
	}
	if err := writer.WriteField("config", string(configJSON)); err != nil {
		return fmt.Errorf("speechmatics: write config part: %w", err)
	}
	return nil
}

// maxUploadFilenameBytes caps the filename written into the part header. A
// media filename is user-controlled and only ever a hint to the provider's
// format detection, so it is truncated rather than rejected.
const maxUploadFilenameBytes = 120

// safeUploadFilename reduces a caller-supplied filename to a bare basename fit
// for a Content-Disposition header. It returns "" when nothing usable remains,
// leaving the fallback to the caller.
//
// Both separators are stripped: the name comes from a media row that may have
// been uploaded from Windows, and `..\..\x` must not reach the header as a
// path. Control characters and quotes go too — those are what would let a
// crafted name forge extra header lines or break out of the quoted-string.
// filepath.Base is deliberately not used: it only knows the host's separator,
// and the host is Linux while the filename may not be.
func safeUploadFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '\\':
			return '/' // one separator to scan for below
		case r == '"' || unicode.IsControl(r):
			return '_'
		default:
			return r
		}
	}, name)

	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	if len(name) > maxUploadFilenameBytes {
		name = truncateRunes(name, maxUploadFilenameBytes)
	}
	return name
}

// truncateRunes cuts s to at most n bytes without splitting a rune.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// safeContentType returns a header-safe media type, dropping any parameters and
// falling back to application/octet-stream. Parsing rather than passing the raw
// value through is what keeps a malformed stored content type from becoming a
// malformed part header.
func safeContentType(raw string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil || mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}

// checkStatus maps HTTP status codes to domain errors. ErrInvalidInput is
// permanent; everything else is retried.
//
// Success is ANY 2xx, not 200: Speechmatics answers a job submission with 201
// Created and a deletion with 200 (both verified live 2026-08-30). Narrowing
// this to 200 would fail every submission.
func (c *Client) checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("speechmatics: unauthorized: %w", domain.ErrUnauthorized)
	case http.StatusTooManyRequests:
		// Retryable: River re-runs the job, and the client-side limiter plus
		// the poll interval already spread the retries out. Retry-After is
		// logged rather than slept on, so no worker slot is held idle.
		c.logger.Warn("Speechmatics rate limited",
			"retry_after_seconds", retryAfterSeconds(resp))
		return fmt.Errorf("speechmatics: rate limited: %w", domain.ErrRateLimited)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return fmt.Errorf("speechmatics: request rejected: %w", domain.ErrInvalidInput)
	default:
		return fmt.Errorf("speechmatics: unexpected status %d: %w", resp.StatusCode, domain.ErrInternal)
	}
}

// retryAfterSeconds reads a numeric Retry-After header, returning 0 when absent
// or in the HTTP-date form.
func retryAfterSeconds(resp *http.Response) int {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

// decodeJSON decodes a bounded response body.
func decodeJSON(body io.Reader, limit int64, dst interface{}) error {
	return json.NewDecoder(io.LimitReader(body, limit)).Decode(dst)
}

// closeBody drains and closes a response body so the connection can be reused.
func closeBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes)) //nolint:errcheck // best-effort drain
	_ = resp.Body.Close()                                                    //nolint:errcheck // best-effort close
}
