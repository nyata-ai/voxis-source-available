package clamav

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantClean  bool
		wantThreat string
		wantErr    bool
	}{
		{
			name:      "clean response",
			raw:       "stream: OK",
			wantClean: true,
		},
		{
			name:      "clean response with null terminator",
			raw:       "stream: OK\x00",
			wantClean: true,
		},
		{
			name:      "clean response with newline",
			raw:       "stream: OK\n",
			wantClean: true,
		},
		{
			name:       "virus found",
			raw:        "stream: Win.Trojan.Agent-123 FOUND",
			wantClean:  false,
			wantThreat: "Win.Trojan.Agent-123",
		},
		{
			name:       "virus found with null terminator",
			raw:        "stream: Eicar-Signature FOUND\x00",
			wantClean:  false,
			wantThreat: "Eicar-Signature",
		},
		{
			name:       "virus found with newline",
			raw:        "stream: ClamAV-Test-File FOUND\n",
			wantClean:  false,
			wantThreat: "ClamAV-Test-File",
		},
		{
			name:    "error response",
			raw:     "INSTREAM size limit exceeded. ERROR",
			wantErr: true,
		},
		{
			// AlertExceedsMax=yes phrases a tripped scan limit as a detection.
			// The scan never completed, so it must be an error: neither a clean
			// verdict nor an accusation against the file.
			name:    "limits exceeded is an incomplete scan, not malware",
			raw:     "stream: Heuristics.Limits.Exceeded FOUND",
			wantErr: true,
		},
		{
			name:    "limits exceeded names the specific limit",
			raw:     "stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND\x00",
			wantErr: true,
		},
		{
			name:    "empty response",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "unknown response",
			raw:     "some garbage",
			wantErr: true,
		},
	}

	scanner := New(slog.Default())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := scanner.parseResponse(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantClean, result.Clean)
			assert.Equal(t, tt.wantThreat, result.ThreatName)
		})
	}
}

// End to end against a fake clamd: an oversized stream must surface as an
// error, so ScanMediaWorker records scan_error and the media stays blocked.
func TestScan_LimitsExceededIsAnError(t *testing.T) {
	t.Parallel()

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		// Drain the INSTREAM command and framed chunks up to the client's
		// zero-length terminator, then answer the way clamd does with
		// AlertExceedsMax=yes. The read loop is bounded: this payload is one chunk.
		buf := make([]byte, 512)
		var got []byte
		for range 16 {
			n, readErr := conn.Read(buf)
			got = append(got, buf[:n]...)
			if readErr != nil {
				return
			}
			if bytes.HasSuffix(got, []byte{0, 0, 0, 0}) {
				break
			}
		}
		_, _ = conn.Write([]byte("stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND\x00"))
	}()

	scanner := New(slog.Default(), WithAddress(ln.Addr().String()))
	result, err := scanner.Scan(context.Background(), bytes.NewReader([]byte("oversized")))

	require.Error(t, err)
	assert.Nil(t, result, "an incomplete scan must not produce a verdict")
	assert.Contains(t, err.Error(), "Heuristics.Limits.Exceeded.MaxFileSize")
}

func TestAvailable_PongResponse(t *testing.T) {
	t.Parallel()

	// Start a mock TCP server that responds to PING with PONG.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 64)
		n, readErr := conn.Read(buf)
		if readErr != nil {
			return
		}

		// Verify we received zPING\x00 — respond with null-terminated PONG
		// matching real clamd z-protocol behavior.
		if string(buf[:n]) == "zPING\x00" {
			_, _ = conn.Write([]byte("PONG\x00"))
		}
	}()

	logger := slog.Default()
	scanner := New(logger, WithAddress(ln.Addr().String()))

	available := scanner.Available(context.Background())
	assert.True(t, available)
}

func TestAvailable_NotReachable(t *testing.T) {
	t.Parallel()

	logger := slog.Default()
	// Use an address that will refuse connections.
	scanner := New(logger, WithAddress("127.0.0.1:1"))

	available := scanner.Available(context.Background())
	assert.False(t, available)
}

func TestAvailable_BadResponse(t *testing.T) {
	t.Parallel()

	// Start a mock TCP server that responds with something other than PONG.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("NOT-PONG"))
	}()

	logger := slog.Default()
	scanner := New(logger, WithAddress(ln.Addr().String()))

	available := scanner.Available(context.Background())
	assert.False(t, available)
}
