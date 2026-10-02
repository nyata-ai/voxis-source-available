package clamav

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

const (
	defaultAddr    = "localhost:3310"
	defaultTimeout = 120 * time.Second
	chunkSize      = 65536                                  // 64 KB — balances syscall overhead and memory
	maxStreamSize  = int64(domain.MaxMediaSize) + 1024*1024 // MaxMediaSize + 1 MB overhead
	maxRespSize    = 4096                                   // clamd responses are short

	responseClean       = "stream: OK"
	responseFoundPrefix = "stream: "
	responseFoundSuffix = " FOUND"

	// clamd running with AlertExceedsMax=yes reports input that tripped a scan
	// limit (max file size, recursion depth, scan time) as a heuristic
	// "detection". The scan did not finish, so the verdict is neither clean nor
	// malware — see parseResponse.
	responseLimitsPrefix = "Heuristics.Limits.Exceeded"
)

// Scanner implements port.MalwareScanner using ClamAV's clamd daemon.
// Connects via TCP to clamd's INSTREAM protocol. The connection should
// be localhost-only (Docker bridge). For remote clamd, use TLS or SSH
// tunnel to protect decrypted audio in transit.
type Scanner struct {
	addr    string
	timeout time.Duration
	logger  *slog.Logger
}

// Option configures a Scanner.
type Option func(*Scanner)

// WithAddress sets the clamd TCP address.
func WithAddress(addr string) Option { return func(s *Scanner) { s.addr = addr } }

// WithTimeout sets the scan timeout.
func WithTimeout(d time.Duration) Option { return func(s *Scanner) { s.timeout = d } }

// New creates a ClamAV scanner with the given options.
func New(logger *slog.Logger, opts ...Option) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Scanner{addr: defaultAddr, timeout: defaultTimeout, logger: logger}
	for _, o := range opts {
		o(s)
	}
	return s
}

// closeConn closes a clamd connection, logging any close error. The read path
// has already returned by the time deferred closes run, so a close failure
// cannot affect the scan result — but it is worth surfacing in logs.
func (s *Scanner) closeConn(conn net.Conn) {
	if err := conn.Close(); err != nil {
		s.logger.Warn("close clamd connection", "error", err)
	}
}

// Scan reads from r and returns the scan result using clamd's INSTREAM protocol.
func (s *Scanner) Scan(ctx context.Context, r io.Reader) (*port.ScanResult, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(s.timeout)
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("connect to clamd at %s: %w", s.addr, err)
	}
	defer s.closeConn(conn)

	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	// Send INSTREAM command
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return nil, fmt.Errorf("send INSTREAM command: %w", err)
	}

	if err := streamChunks(conn, r); err != nil {
		return nil, err
	}

	// Send zero-length terminator
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return nil, fmt.Errorf("send terminator: %w", err)
	}

	// Read response (bounded to prevent memory exhaustion)
	var resp bytes.Buffer
	if _, err := io.Copy(&resp, io.LimitReader(conn, maxRespSize)); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return s.parseResponse(resp.String())
}

// streamChunks writes r to conn using clamd's INSTREAM framing:
// [4-byte big-endian length][data] per chunk. The caller sends the
// zero-length terminator.
func streamChunks(conn net.Conn, r io.Reader) error {
	buf := make([]byte, chunkSize)
	limited := io.LimitReader(r, maxStreamSize)
	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			if n > chunkSize {
				// Defensive: Read must not return more than len(buf); guard the
				// int→uint32 conversion below against overflow regardless.
				return fmt.Errorf("read returned %d bytes, exceeds chunk size %d", n, chunkSize)
			}
			var lenBuf [4]byte
			binary.BigEndian.PutUint32(lenBuf[:], uint32(n))
			if _, err := conn.Write(lenBuf[:]); err != nil {
				return fmt.Errorf("write chunk length: %w", err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return fmt.Errorf("write chunk data: %w", err)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read input: %w", readErr)
		}
	}
}

// Available reports whether the clamd daemon is reachable by sending a PING.
func (s *Scanner) Available(ctx context.Context) bool {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return false
	}
	defer s.closeConn(conn)

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return false
	}
	if _, err := conn.Write([]byte("zPING\x00")); err != nil {
		return false
	}

	var resp bytes.Buffer
	if _, err := io.Copy(&resp, io.LimitReader(conn, maxRespSize)); err != nil {
		// Fall through: whatever was read before the error still decides
		// availability, matching clamd deployments that drop the connection
		// right after replying.
		s.logger.Debug("clamd ping read ended with error", "error", err)
	}
	return strings.TrimRight(resp.String(), "\x00\n\r ") == "PONG"
}

// parseResponse maps one clamd reply onto a verdict. Anything it does not
// recognize is an error, which the caller turns into scan_error — the media
// then stays blocked rather than being waved through unscanned.
func (s *Scanner) parseResponse(raw string) (*port.ScanResult, error) {
	resp := strings.TrimRight(raw, "\x00\n\r ")
	if resp == responseClean {
		return &port.ScanResult{Clean: true}, nil
	}
	if strings.HasSuffix(resp, responseFoundSuffix) {
		threat := strings.TrimPrefix(resp, responseFoundPrefix)
		threat = strings.TrimSuffix(threat, responseFoundSuffix)
		if strings.HasPrefix(threat, responseLimitsPrefix) {
			// An incomplete scan, not a detection: clamd gave up at a configured
			// limit. Reporting it as malware would accuse a clean file; reporting
			// it clean would admit bytes nothing looked at.
			s.logger.Error("clamd stopped scanning at a configured limit",
				"limit", threat, "addr", s.addr)
			return nil, fmt.Errorf("clamd scan incomplete: %s", threat)
		}
		return &port.ScanResult{Clean: false, ThreatName: threat}, nil
	}
	return nil, fmt.Errorf("unexpected clamd response: %q", resp)
}
