package speechmatics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/voxis/backend/internal/domain"
	"github.com/voxis/backend/internal/port"
)

// spoolOpts isolates a test client's staging directory and its sweep root from
// the host's real temp dir, so no test can reach a sibling process's spool.
func spoolOpts(t *testing.T) []Option {
	t.Helper()
	return []Option{WithSpoolDir(t.TempDir()), withSpoolRoot(t.TempDir())}
}

// newTestClient builds a client pointed at srv with an isolated spool dir.
func newTestClient(t *testing.T, srv *httptest.Server, opts ...Option) *Client {
	t.Helper()
	base := append(spoolOpts(t), WithHTTPClient(srv.Client()))
	c, err := NewClient(Config{APIURL: srv.URL, APIKey: "test-key", Mode: ModeSaaS}, append(base, opts...)...)
	require.NoError(t, err)
	return c
}

// stage runs Upload and returns the staging token plus the spooled path.
func stage(t *testing.T, c *Client, content string) (token, path string) {
	t.Helper()
	return stageNamed(t, c, content, "audio.wav", "audio/wav")
}

// stageNamed is stage with an explicit filename and content type.
func stageNamed(t *testing.T, c *Client, content, filename, contentType string) (token, path string) {
	t.Helper()
	token, err := c.Upload(context.Background(), strings.NewReader(content), filename, contentType)
	require.NoError(t, err)
	path, err = parseSpoolToken(token, c.spoolDir)
	require.NoError(t, err)
	require.FileExists(t, path)
	return token, path
}

func TestNewClient_RequiresAPIKeyInSaaSMode(t *testing.T) {
	_, err := NewClient(Config{Mode: ModeSaaS})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestNewClient_ContainerModeNeedsNoKey(t *testing.T) {
	c, err := NewClient(Config{Mode: ModeContainer, APIURL: "http://sm.internal"}, spoolOpts(t)...)
	require.NoError(t, err)

	req, err := c.newRequest(context.Background(), http.MethodGet, "/v2/jobs")
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestUpload_StagesLocallyWithoutNetwork(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k"}, spoolOpts(t)...)
	require.NoError(t, err)

	token, path := stage(t, c, "audio-bytes")
	assert.True(t, strings.HasPrefix(token, spoolTokenPrefix))

	content, readErr := os.ReadFile(path) //nolint:gosec // test-owned temp path
	require.NoError(t, readErr)
	assert.Equal(t, "audio-bytes", string(content))
}

func TestUpload_EnforcesSizeCeiling(t *testing.T) {
	dir := t.TempDir()
	// The ceiling is lowered here only so the test does not write 900 MB; the
	// enforcement path is identical.
	c, err := NewClient(Config{APIKey: "k"}, WithSpoolDir(dir), withMaxSpoolBytes(1024))
	require.NoError(t, err)

	// A reader that never ends: the ceiling, not the source, must stop it.
	_, err = c.Upload(context.Background(), infiniteReader{}, "big.wav", "audio/wav")
	require.ErrorIs(t, err, domain.ErrInvalidInput)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "the oversized spool file must be removed")
}

// infiniteReader yields zero bytes forever.
type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) { return len(p), nil }

func TestSubmit_PostsMultipartAndRemovesSpool(t *testing.T) {
	var gotConfig map[string]interface{}
	var gotAudio string
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v2/jobs", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")

		assert.NoError(t, r.ParseMultipartForm(1<<20))
		file, _, fErr := r.FormFile("data_file")
		assert.NoError(t, fErr)
		defer file.Close() //nolint:errcheck // test cleanup
		raw, rErr := io.ReadAll(file)
		assert.NoError(t, rErr)
		gotAudio = string(raw)

		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sm-job-1"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, path := stage(t, c, "wav-bytes")

	jobID, err := c.Submit(context.Background(), port.TranscriptionRequest{
		AudioURL:    token,
		Diarization: true,
		Languages:   []string{"id", "zh"},
		Reference:   "trans-42",
	})
	require.NoError(t, err)
	assert.Equal(t, "sm-job-1", jobID)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "wav-bytes", gotAudio)

	assert.NoFileExists(t, path, "the staged plaintext must not outlive the submission")

	assert.Equal(t, "transcription", gotConfig["type"])
	tc := gotConfig["transcription_config"].(map[string]interface{})
	assert.Equal(t, "melia-1", tc["model"])
	assert.Equal(t, "multi", tc["language"])
	assert.Equal(t, "speaker", tc["diarization"])
	// "zh" maps to Speechmatics' Mandarin code; hints are sorted.
	assert.Equal(t, []interface{}{"cmn", "id"}, tc["language_hints"])
	assert.Equal(t, "trans-42", gotConfig["tracking"].(map[string]interface{})["reference"])
}

func TestSubmit_OmitsDiarizationAndHintsWhenUnset(t *testing.T) {
	var gotConfig map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		_, _ = w.Write([]byte(`{"id":"sm-job-2"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stage(t, c, "x")

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.NoError(t, err)

	tc := gotConfig["transcription_config"].(map[string]interface{})
	assert.NotContains(t, tc, "diarization")
	assert.NotContains(t, tc, "language_hints")
	assert.NotContains(t, gotConfig, "tracking")
}

// submitAndCaptureConfig submits a minimal job against a fresh test server and
// returns the decoded transcription_config map. Used by the
// speaker_diarization_config tests below, which only vary Config and
// req.Diarization.
func submitAndCaptureConfig(t *testing.T, cfg Config, diarization bool) map[string]interface{} {
	t.Helper()
	return submitAndCaptureConfigWithSpeakers(t, cfg, diarization, 0)
}

// submitAndCaptureConfigWithSpeakers is submitAndCaptureConfig with a declared
// speaker count (0 = the submitter left it on auto-detect).
func submitAndCaptureConfigWithSpeakers(t *testing.T, cfg Config, diarization bool, declared int) map[string]interface{} {
	t.Helper()
	return submitAndCaptureConfigForRequest(t, cfg, port.TranscriptionRequest{
		Diarization:      diarization,
		ExpectedSpeakers: declared,
	}, nil)
}

// submitAndCaptureConfigForRequest submits req (AudioURL is filled in with a
// freshly staged token) and returns the decoded transcription_config map.
// logger, when non-nil, replaces the client's default logger.
func submitAndCaptureConfigForRequest(
	t *testing.T,
	cfg Config,
	req port.TranscriptionRequest,
	logger *slog.Logger,
) map[string]interface{} {
	t.Helper()
	var gotConfig map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		_, _ = w.Write([]byte(`{"id":"sm-job-tuning"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	cfg.APIURL = srv.URL
	cfg.APIKey = "test-key"
	cfg.Mode = ModeSaaS
	opts := append(spoolOpts(t), WithHTTPClient(srv.Client()))
	if logger != nil {
		opts = append(opts, WithLogger(logger))
	}
	c, err := NewClient(cfg, opts...)
	require.NoError(t, err)

	token, _ := stage(t, c, "x")
	req.AudioURL = token
	_, err = c.Submit(context.Background(), req)
	require.NoError(t, err)

	tc, ok := gotConfig["transcription_config"].(map[string]interface{})
	require.True(t, ok)
	return tc
}

func TestSubmit_SpeakerDiarizationConfigBothFieldsWhenDiarizationOn(t *testing.T) {
	sensitivity := 0.3
	preferCurrent := true
	tc := submitAndCaptureConfig(t, Config{SpeakerSensitivity: &sensitivity, PreferCurrentSpeaker: &preferCurrent}, true)

	diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
	require.True(t, ok, "expected speaker_diarization_config in %v", tc)
	assert.Equal(t, map[string]interface{}{
		"speaker_sensitivity":    0.3,
		"prefer_current_speaker": true,
	}, diarCfg)
}

func TestSubmit_SpeakerDiarizationConfigAbsentWhenKnobsUnset(t *testing.T) {
	tc := submitAndCaptureConfig(t, Config{}, true)
	assert.NotContains(t, tc, "speaker_diarization_config")
}

func TestSubmit_SpeakerDiarizationConfigAbsentWhenDiarizationOffEvenWithKnobsSet(t *testing.T) {
	sensitivity := 0.3
	preferCurrent := true
	tc := submitAndCaptureConfig(t, Config{SpeakerSensitivity: &sensitivity, PreferCurrentSpeaker: &preferCurrent}, false)
	assert.NotContains(t, tc, "speaker_diarization_config")
	assert.NotContains(t, tc, "diarization")
}

func TestSubmit_SpeakerDiarizationConfigSensitivityOnly(t *testing.T) {
	sensitivity := 0.7
	tc := submitAndCaptureConfig(t, Config{SpeakerSensitivity: &sensitivity}, true)

	diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
	require.True(t, ok, "expected speaker_diarization_config in %v", tc)
	assert.Equal(t, map[string]interface{}{"speaker_sensitivity": 0.7}, diarCfg)
}

func TestSubmit_SpeakerDiarizationConfigPreferCurrentOnly(t *testing.T) {
	preferCurrent := false
	tc := submitAndCaptureConfig(t, Config{PreferCurrentSpeaker: &preferCurrent}, true)

	diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
	require.True(t, ok, "expected speaker_diarization_config in %v", tc)
	assert.Equal(t, map[string]interface{}{"prefer_current_speaker": false}, diarCfg)
}

// The batch API has no max_speakers knob, so a declared count of 1-2 is
// honored by dropping speaker_sensitivity to 0.2. Above 2 the deployment's
// configured value must survive untouched.
func TestSubmit_DeclaredFewSpeakersLowersSensitivity(t *testing.T) {
	sensitivity := 0.4
	preferCurrent := true
	for _, declared := range []int{1, 2} {
		tc := submitAndCaptureConfigWithSpeakers(t,
			Config{SpeakerSensitivity: &sensitivity, PreferCurrentSpeaker: &preferCurrent}, true, declared)

		diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
		require.True(t, ok, "expected speaker_diarization_config for %d speakers in %v", declared, tc)
		assert.Equal(t, map[string]interface{}{
			"speaker_sensitivity":    0.2,
			"prefer_current_speaker": true,
		}, diarCfg, "declared=%d", declared)
	}
}

func TestSubmit_DeclaredFewSpeakersEmitsBlockWithNoConfiguredKnobs(t *testing.T) {
	tc := submitAndCaptureConfigWithSpeakers(t, Config{}, true, 2)

	diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
	require.True(t, ok, "expected speaker_diarization_config in %v", tc)
	assert.Equal(t, map[string]interface{}{"speaker_sensitivity": 0.2}, diarCfg)
}

func TestSubmit_DeclaredManySpeakersKeepsConfiguredSensitivity(t *testing.T) {
	sensitivity := 0.4
	tc := submitAndCaptureConfigWithSpeakers(t, Config{SpeakerSensitivity: &sensitivity}, true, 3)

	diarCfg, ok := tc["speaker_diarization_config"].(map[string]interface{})
	require.True(t, ok, "expected speaker_diarization_config in %v", tc)
	assert.Equal(t, map[string]interface{}{"speaker_sensitivity": 0.4}, diarCfg)
}

func TestSubmit_DeclaredManySpeakersWithNoKnobsOmitsBlock(t *testing.T) {
	tc := submitAndCaptureConfigWithSpeakers(t, Config{}, true, 3)
	assert.NotContains(t, tc, "speaker_diarization_config")
}

func TestSubmit_DeclaredSpeakersIgnoredWhenDiarizationOff(t *testing.T) {
	tc := submitAndCaptureConfigWithSpeakers(t, Config{}, false, 2)
	assert.NotContains(t, tc, "speaker_diarization_config")
	assert.NotContains(t, tc, "diarization")
}

// A declaration this provider cannot apply must leave a trace: the user picked
// a number and got provider-default diarization.
func TestSubmit_WarnsWhenDeclaredCountCannotBeApplied(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	submitAndCaptureConfigForRequest(t, Config{},
		port.TranscriptionRequest{Diarization: true, ExpectedSpeakers: 5, Reference: "trans-77"},
		logger)

	out := logs.String()
	assert.Contains(t, out, "cannot constrain diarization")
	assert.Contains(t, out, "expected_speakers=5")
	assert.Contains(t, out, "trans-77")
}

// One or two speakers IS applied (as a sensitivity drop), so it must not warn.
func TestSubmit_DoesNotWarnWhenDeclaredCountIsApplied(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	submitAndCaptureConfigForRequest(t, Config{},
		port.TranscriptionRequest{Diarization: true, ExpectedSpeakers: 2, Reference: "trans-78"},
		logger)

	assert.NotContains(t, logs.String(), "cannot constrain diarization")
}

// Without diarization there is no diarization behavior to warn about.
func TestSubmit_DoesNotWarnAboutSpeakerCountWithoutDiarization(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	submitAndCaptureConfigForRequest(t, Config{},
		port.TranscriptionRequest{Diarization: false, ExpectedSpeakers: 5, Reference: "trans-79"},
		logger)

	assert.NotContains(t, logs.String(), "cannot constrain diarization")
}

func TestSubmit_FallsBackToDeploymentLanguageHints(t *testing.T) {
	var gotConfig map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		_, _ = w.Write([]byte(`{"id":"sm-job-3"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c, err := NewClient(
		Config{APIURL: srv.URL, APIKey: "k", Mode: ModeSaaS, LanguageHints: []string{"id", "en"}},
		append(spoolOpts(t), WithHTTPClient(srv.Client()))...)
	require.NoError(t, err)

	token, _ := stage(t, c, "x")
	_, err = c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.NoError(t, err)

	tc := gotConfig["transcription_config"].(map[string]interface{})
	assert.Equal(t, []interface{}{"id", "en"}, tc["language_hints"])
}

// jv has no Melia language at all (see voxisToMeliaLanguage); it must be
// dropped from the request rather than forwarded and rejected by the
// provider as a bad hint.
func TestSubmit_DropsUnsupportedLanguageHintsButKeepsSupportedOnes(t *testing.T) {
	var gotConfig map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		_, _ = w.Write([]byte(`{"id":"sm-job-jv"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stage(t, c, "x")

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{
		AudioURL:  token,
		Languages: []string{"jv", "id"},
	})
	require.NoError(t, err)

	tc := gotConfig["transcription_config"].(map[string]interface{})
	assert.Equal(t, []interface{}{"id"}, tc["language_hints"])
}

// A dropped hint is worth a log line — the transcript goes out with a
// different language bias than the user picked, silently otherwise.
func TestSubmit_WarnsWhenLanguageHintsAreDropped(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	submitAndCaptureConfigForRequest(t, Config{},
		port.TranscriptionRequest{Languages: []string{"jv", "su", "id"}, Reference: "trans-80"},
		logger)

	out := logs.String()
	assert.Contains(t, out, "does not support some requested language hints")
	assert.Contains(t, out, "trans-80")
	assert.Contains(t, out, "jv")
	assert.Contains(t, out, "su")
}

func TestSubmit_RejectsForeignStagingToken(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k"}, spoolOpts(t)...)
	require.NoError(t, err)

	_, err = c.Submit(context.Background(), port.TranscriptionRequest{
		AudioURL: "https://foreign.example/uploaded/abc",
	})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestSubmit_UnsupportedLanguageIsPermanentAndDropsSpool(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k"}, spoolOpts(t)...)
	require.NoError(t, err)

	token, path := stage(t, c, "x")
	_, err = c.Submit(context.Background(), port.TranscriptionRequest{
		AudioURL:  token,
		Languages: []string{"xx"},
	})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.NoFileExists(t, path, "an early return must still remove the staged plaintext")
}

func TestSubmit_ErrorStatusesMapToDomainErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		wantErr error
	}{
		{"unauthorized", http.StatusUnauthorized, nil, domain.ErrUnauthorized},
		{"bad request", http.StatusBadRequest, nil, domain.ErrInvalidInput},
		{"rate limited", http.StatusTooManyRequests, map[string]string{"Retry-After": "7"}, domain.ErrRateLimited},
		{"server error", http.StatusBadGateway, nil, domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":"SYNTHETIC_PROVIDER_ERROR_MARKER"}`)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			c := newTestClient(t, srv)
			token, path := stage(t, c, "x")

			_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
			require.ErrorIs(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PROVIDER_ERROR_MARKER")
			assert.NoFileExists(t, path)
		})
	}
}

func TestSubmit_EmptyJobIDIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stage(t, c, "x")

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.ErrorIs(t, err, domain.ErrInternal)
}

func TestGetStatus_RunningMapsToProcessing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/jobs/sm-1", r.URL.Path)
		assert.Equal(t, "0", r.URL.Query().Get("wait"), "the status GET must disable the server-side long-poll")
		_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"running"}}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
	require.NoError(t, err)
	assert.Equal(t, "processing", result.Status)
}

func TestGetStatus_DoneFetchesAndMapsTranscript(t *testing.T) {
	transcript, err := os.ReadFile(filepath.Join("testdata", "code_switched.json"))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/jobs/sm-1":
			assert.Equal(t, "0", r.URL.Query().Get("wait"), "the status GET must disable the server-side long-poll")
			_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"done"}}`)) //nolint:errcheck // test server
		case "/v2/jobs/sm-1/transcript":
			assert.Equal(t, "json-v2", r.URL.Query().Get("format"))
			assert.Equal(t, "0", r.URL.Query().Get("wait"), "the transcript GET must disable the server-side long-poll")
			_, _ = w.Write(transcript) //nolint:errcheck // test server
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
	require.NoError(t, err)
	assert.Equal(t, "done", result.Status)
	assert.Equal(t, "Jadi kita review kontraknya. Okay, siap.", result.FullTranscript)
	assert.Equal(t, 2, result.SpeakerCount)
}

func TestGetStatus_RejectedAndExpiredMapToError(t *testing.T) {
	for _, status := range []string{"rejected", "expired"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				body := fmt.Sprintf(`{"job":{"id":"sm-1","status":%q,"errors":[{"message":"SYNTHETIC_PROVIDER_ERROR_MARKER"}]}}`, status)
				_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
			require.NoError(t, err)
			assert.Equal(t, "error", result.Status)
			assert.Equal(t, "Speechmatics job "+status, result.ErrorMessage)
			assert.NotContains(t, result.ErrorMessage, "SYNTHETIC_PROVIDER_ERROR_MARKER")
		})
	}
}

// An unrecognized status must reach the poller VERBATIM. Mapping it to "error"
// is terminal there: the poller marks the transcription failed and then deletes
// the provider job, which still holds the only copy of the audio. Passing the
// raw string through lands in the poller's default branch, which only logs.
func TestGetStatus_UnknownStatusPassesThroughUnchanged(t *testing.T) {
	for _, status := range []string{"teleported", "deleted", "on_hold"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				body := fmt.Sprintf(`{"job":{"id":"sm-1","status":%q}}`, status)
				_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
			require.NoError(t, err)
			assert.Equal(t, status, result.Status)
			assert.NotEqual(t, "error", result.Status, "an unknown status must never be terminal")
			assert.NotEqual(t, "done", result.Status)
			assert.Empty(t, result.ErrorMessage)
		})
	}
}

// A 200 that does not decode into {"job":{"status":...}} is a broken fetch, not
// a status. Returning any result at all would let a proxy error page or a
// truncated body drive a terminal decision.
func TestGetStatus_UnusableStatusBodyIsAFetchError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty status", `{"job":{"id":"sm-1","status":""}}`},
		{"status key missing", `{"job":{"id":"sm-1"}}`},
		{"job envelope missing", `{"id":"sm-1","status":"done"}`},
		{"empty object", `{}`},
		{"null body", `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
			require.Error(t, err)
			assert.Nil(t, result, "a broken fetch must never yield a result")
			assert.ErrorIs(t, err, domain.ErrInternal)
		})
	}
}

// The whole point of the two fixes above: a status the adapter does not
// recognize must not reach a code path that deletes the provider's job.
func TestGetStatus_NeverDeletesOnAnUnknownStatus(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			sawDelete = true
		}
		_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"deleted"}}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
	require.NoError(t, err)
	assert.Equal(t, "deleted", result.Status)
	assert.False(t, sawDelete)
}

// A "done" job whose transcript body is not a transcript at all must surface as
// an error, not as a completed blank transcript: completing would let the
// poller delete the provider's only copy of the audio.
func TestGetStatus_DoneWithAnUnusableTranscriptIsAnError(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"results":[]}`} {
		t.Run(body, func(t *testing.T) {
			result, err := newTestClient(t, transcriptServer(t, body)).
				GetStatus(context.Background(), "sm-1")
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, domain.ErrInternal)
		})
	}
}

// A valid job envelope with zero results is the provider's honest "no speech"
// answer (verified live 2026-08-30 on a silent clip: status "done", duration 8,
// results []). It completes as an empty transcript — erroring made every silent
// or music-only upload a job the poller retried forever.
func TestGetStatus_DoneWithNoSpeechCompletesEmpty(t *testing.T) {
	body := `{"job":{"id":"sm-1","duration":8},"results":[]}`

	result, err := newTestClient(t, transcriptServer(t, body)).
		GetStatus(context.Background(), "sm-1")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "done", result.Status)
	assert.Empty(t, result.FullTranscript)
	assert.Equal(t, 0, result.WordCount)
	assert.JSONEq(t, `[]`, string(result.Utterances))
	assert.InDelta(t, 8.0, result.AudioDuration, 0.001)
}

// transcriptServer answers the status probe with a done job and the transcript
// fetch with body.
func transcriptServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transcript") {
			_, _ = w.Write([]byte(body)) //nolint:errcheck // test server
			return
		}
		_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"done"}}`)) //nolint:errcheck // test server
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGetStatus_TransportErrorsPropagate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetStatus(context.Background(), "sm-1")
	require.ErrorIs(t, err, domain.ErrInternal)
}

func TestGetStatus_EmptyJobIDRejected(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k"}, spoolOpts(t)...)
	require.NoError(t, err)

	_, err = c.GetStatus(context.Background(), "")
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestWaitForResult_DoneFetchesAndMapsTranscript(t *testing.T) {
	transcript, err := os.ReadFile(filepath.Join("testdata", "code_switched.json"))
	require.NoError(t, err)

	var gotWait string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/jobs/sm-1":
			gotWait = r.URL.Query().Get("wait")
			_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"done"}}`)) //nolint:errcheck // test server
		case "/v2/jobs/sm-1/transcript":
			assert.Equal(t, "json-v2", r.URL.Query().Get("format"))
			_, _ = w.Write(transcript) //nolint:errcheck // test server
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv).WaitForResult(context.Background(), "sm-1", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "30", gotWait)
	assert.Equal(t, "done", result.Status)
	assert.Equal(t, "Jadi kita review kontraknya. Okay, siap.", result.FullTranscript)
}

func TestWaitForResult_RunningMapsToProcessing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/jobs/sm-1", r.URL.Path)
		_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"running"}}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	result, err := newTestClient(t, srv).WaitForResult(context.Background(), "sm-1", 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "processing", result.Status)
}

// The wait value is clamped: a sub-second request must not become "?wait=0"
// (which is a plain status probe) and a long one must not exceed the cap.
func TestWaitForResult_ClampsTheWaitParameter(t *testing.T) {
	tests := []struct {
		name     string
		maxWait  time.Duration
		wantWait string
	}{
		{"below the floor", 100 * time.Millisecond, "1"},
		{"zero", 0, "1"},
		{"negative", -time.Second, "1"},
		{"above the cap", 10 * time.Minute, "60"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotWait string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotWait = r.URL.Query().Get("wait")
				_, _ = w.Write([]byte(`{"job":{"id":"sm-1","status":"running"}}`)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			_, err := newTestClient(t, srv).WaitForResult(context.Background(), "sm-1", tt.maxWait)
			require.NoError(t, err)
			assert.Equal(t, tt.wantWait, gotWait)
		})
	}
}

func TestWaitForResult_EmptyJobIDRejected(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k"}, spoolOpts(t)...)
	require.NoError(t, err)

	_, err = c.WaitForResult(context.Background(), "", time.Second)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestDelete_TreatsNotFoundAsSuccess(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"accepted", http.StatusOK, nil},
		{"already gone", http.StatusNotFound, nil},
		{"unauthorized", http.StatusUnauthorized, domain.ErrUnauthorized},
		{"server error", http.StatusInternalServerError, domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			err := newTestClient(t, srv).Delete(context.Background(), "sm-1")
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestConfirmDeleted_UsesProviderDeletionContract(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"404 confirms deletion", http.StatusNotFound, nil},
		{"200 means it still exists", http.StatusOK, domain.ErrConflict},
		{"401 surfaces as-is", http.StatusUnauthorized, domain.ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "0", r.URL.Query().Get("wait"), "the ConfirmDeleted GET must disable the server-side long-poll")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"job":{"id":"sm-1"}}`)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			err := newTestClient(t, srv).ConfirmDeleted(context.Background(), "sm-1")
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// CheckAuth is the startup probe: it must distinguish a rejected credential
// (or a region that rejects valid ones, as eu2 does) from a merely unreachable
// endpoint, because the caller fails startup on the first and only warns on the
// second.
func TestCheckAuth_ClassifiesTheProbeOutcome(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"200 accepts the credential", http.StatusOK, nil},
		{"401 is a rejected credential", http.StatusUnauthorized, domain.ErrUnauthorized},
		{"403 is a rejected credential", http.StatusForbidden, domain.ErrUnauthorized},
		{"500 is inconclusive, not a verdict", http.StatusInternalServerError, domain.ErrInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				gotPath = r.URL.Path + "?" + r.URL.RawQuery
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"jobs":[]}`)) //nolint:errcheck // test server
			}))
			defer srv.Close()

			err := newTestClient(t, srv).CheckAuth(context.Background())
			// One page of the job list is the cheapest authenticated call.
			assert.Equal(t, "/v2/jobs?limit=1", gotPath)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// The spool directory is minted per process, so its name cannot be guessed and
// pre-created (or symlinked) by a local attacker before the service starts.
func TestNewClient_MintsAnUnpredictablePrivateSpoolDir(t *testing.T) {
	root := t.TempDir()

	first, err := NewClient(Config{APIKey: "k"}, withSpoolRoot(root))
	require.NoError(t, err)
	second, err := NewClient(Config{APIKey: "k"}, withSpoolRoot(root))
	require.NoError(t, err)

	assert.NotEqual(t, first.spoolDir, second.spoolDir, "two processes must not share a spool dir")
	for _, c := range []*Client{first, second} {
		assert.Equal(t, root, filepath.Dir(c.spoolDir))
		assert.True(t, strings.HasPrefix(filepath.Base(c.spoolDir), spoolDirPrefix),
			"the global temp sweeper matches on this prefix")

		info, statErr := os.Stat(c.spoolDir)
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestNewClient_SweepsStaleSiblingSpoolDirs(t *testing.T) {
	root := t.TempDir()

	// A dead process's directory, older than the one-hour cutoff.
	deadDir := filepath.Join(root, spoolDirPrefix+"dead")
	require.NoError(t, os.Mkdir(deadDir, 0o700))
	deadFile := filepath.Join(deadDir, "audio-123.bin")
	require.NoError(t, os.WriteFile(deadFile, []byte("plaintext"), 0o600))

	// A sibling that started moments ago: it may hold an in-flight submission.
	liveDir := filepath.Join(root, spoolDirPrefix+"live")
	require.NoError(t, os.Mkdir(liveDir, 0o700))
	liveFile := filepath.Join(liveDir, "audio-456.bin")
	require.NoError(t, os.WriteFile(liveFile, []byte("in flight"), 0o600))

	// Something else's old directory that happens to live in the same temp dir.
	strangerDir := filepath.Join(root, "someone-elses-cache")
	require.NoError(t, os.Mkdir(strangerDir, 0o700))
	strangerFile := filepath.Join(strangerDir, "audio-789.bin")
	require.NoError(t, os.WriteFile(strangerFile, []byte("not ours"), 0o600))

	old := time.Now().Add(-2 * time.Hour)
	for _, dir := range []string{deadDir, strangerDir} {
		require.NoError(t, os.Chtimes(dir, old, old))
	}

	c, err := NewClient(Config{APIKey: "k"}, withSpoolRoot(root))
	require.NoError(t, err)

	assert.NoFileExists(t, deadFile, "a dead process cannot have in-flight submissions")
	assert.NoDirExists(t, deadDir, "the emptied directory goes too")
	assert.FileExists(t, liveFile, "a sibling younger than the cutoff is left alone")
	assert.FileExists(t, strangerFile, "only voxis-sm-spool-* directories are swept")
	assert.DirExists(t, c.spoolDir, "the sweep must not reap this process's own dir")
}

func TestSweepStaleSpoolDirs_LeavesUnexpectedContentAlone(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, spoolDirPrefix+"mixed")
	require.NoError(t, os.Mkdir(dir, 0o700))

	spooled := filepath.Join(dir, "audio-1.bin")
	foreign := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(spooled, []byte("audio"), 0o600))
	require.NoError(t, os.WriteFile(foreign, []byte("someone else's"), 0o600))

	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(dir, old, old))

	sweepStaleSpoolDirs(root, filepath.Join(root, "self"), staleSpoolDirMaxAge, nil)

	assert.NoFileExists(t, spooled, "spool-shaped files are removed")
	assert.FileExists(t, foreign, "anything else is left in place")
	assert.DirExists(t, dir, "so the non-empty directory survives too")
}

// The staging token round-trips through a database column before Submit reads
// it back, so it must prove it names a file this client staged. Without that,
// a crafted AudioURL turns Submit into an arbitrary open-and-delete.
func TestParseSpoolToken_RejectsAnythingOutsideTheSpoolDir(t *testing.T) {
	spoolDir := t.TempDir()
	elsewhere := t.TempDir()

	valid := filepath.Join(spoolDir, "audio-abc.bin")
	got, err := parseSpoolToken(spoolToken(valid), spoolDir)
	require.NoError(t, err)
	assert.Equal(t, valid, got)

	tests := []struct {
		name  string
		token string
	}{
		{"no prefix", "/etc/passwd"},
		{"empty path", spoolTokenPrefix},
		{"another directory", spoolToken(filepath.Join(elsewhere, "audio-abc.bin"))},
		{"absolute escape", spoolToken("/etc/passwd")},
		{"traversal out and back", spoolToken(filepath.Join(spoolDir, "..", "audio-abc.bin"))},
		{"nested subdirectory", spoolToken(filepath.Join(spoolDir, "sub", "audio-abc.bin"))},
		{"wrong basename prefix", spoolToken(filepath.Join(spoolDir, "secret.bin"))},
		{"wrong basename suffix", spoolToken(filepath.Join(spoolDir, "audio-abc.key"))},
		{"the spool dir itself", spoolToken(spoolDir)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSpoolToken(tt.token, spoolDir)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
}

// Speechmatics detects the container format from the part headers, so the
// original filename and content type have to survive the Upload -> Submit hop.
func TestSubmit_DataFilePartCarriesTheOriginalNameAndType(t *testing.T) {
	var gotFilename, gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, mErr := r.MultipartReader()
		assert.NoError(t, mErr)
		for {
			part, pErr := reader.NextPart()
			if pErr != nil {
				break
			}
			if part.FormName() == "data_file" {
				gotFilename = part.FileName()
				gotContentType = part.Header.Get("Content-Type")
			}
			_, _ = io.Copy(io.Discard, part) //nolint:errcheck // test server
		}
		_, _ = w.Write([]byte(`{"id":"sm-job-9"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stageNamed(t, c, "opus-bytes", "Rapat Notaris.ogg", "audio/ogg")

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.NoError(t, err)

	assert.Equal(t, "Rapat Notaris.ogg", gotFilename, "the extension is what drives format detection")
	assert.Equal(t, "audio/ogg", gotContentType)
}

func TestSubmit_SanitizesTheFilenameAndContentType(t *testing.T) {
	var gotFilename, gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, mErr := r.MultipartReader()
		assert.NoError(t, mErr)
		for {
			part, pErr := reader.NextPart()
			if pErr != nil {
				break
			}
			if part.FormName() == "data_file" {
				gotFilename = part.FileName()
				gotContentType = part.Header.Get("Content-Type")
			}
			_, _ = io.Copy(io.Discard, part) //nolint:errcheck // test server
		}
		_, _ = w.Write([]byte(`{"id":"sm-job-10"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stageNamed(t, c, "x", `..\..\Windows\evil".wav`, "not a media type")

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.NoError(t, err)

	assert.Equal(t, "evil_.wav", gotFilename)
	assert.Equal(t, "application/octet-stream", gotContentType)
}

func TestSafeUploadFilename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"rapat.wav", "rapat.wav"},
		{"Rapat Notaris 2026.m4a", "Rapat Notaris 2026.m4a"},
		{"/var/lib/secret.wav", "secret.wav"},
		{`C:\Users\x\audio.mp3`, "audio.mp3"},
		{`../../etc/passwd`, "passwd"},
		{"say\"hi\".wav", "say_hi_.wav"},
		{"line\nbreak.wav", "line_break.wav"},
		{"", ""},
		{"/", ""},
		{"..", ""},
		{strings.Repeat("a", 400) + ".wav", strings.Repeat("a", maxUploadFilenameBytes)},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := safeUploadFilename(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, `"`)
			assert.NotContains(t, got, "\n")
			assert.LessOrEqual(t, len(got), maxUploadFilenameBytes)
		})
	}
}

// The metadata map must not outlive the submission it belongs to.
func TestSubmit_ClearsPendingUploadMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"sm-job-11"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	token, path := stage(t, c, "x")
	require.Len(t, c.pending, 1)

	_, err := c.Submit(context.Background(), port.TranscriptionRequest{AudioURL: token})
	require.NoError(t, err)
	assert.Empty(t, c.pending, "a successful submission releases the entry")
	assert.NoFileExists(t, path)

	// An early error path releases it too: an unsupported language never
	// reaches the network.
	token, path = stage(t, c, "x")
	require.Len(t, c.pending, 1)
	_, err = c.Submit(context.Background(), port.TranscriptionRequest{
		AudioURL:  token,
		Languages: []string{"xx"},
	})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Empty(t, c.pending, "a failed submission releases the entry too")
	assert.NoFileExists(t, path)
}

// submitAndCaptureWholeConfig submits req (AudioURL is filled in with a freshly
// staged token) and returns the WHOLE decoded job config, not just its
// transcription_config — notification_config is a top-level block.
func submitAndCaptureWholeConfig(t *testing.T, req port.TranscriptionRequest) map[string]interface{} {
	t.Helper()
	var gotConfig map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		assert.NoError(t, json.Unmarshal([]byte(r.FormValue("config")), &gotConfig))
		_, _ = w.Write([]byte(`{"id":"sm-job-notify"}`)) //nolint:errcheck // test server
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	token, _ := stage(t, c, "x")
	req.AudioURL = token
	_, err := c.Submit(context.Background(), req)
	require.NoError(t, err)
	return gotConfig
}

// The notification is a wake-up signal, so contents must be EMPTY: an empty
// body means no transcript is ever pushed back to us over the callback.
func TestSubmit_EmitsNotificationConfigWhenCallbackEnabled(t *testing.T) {
	got := submitAndCaptureWholeConfig(t, port.TranscriptionRequest{
		CallbackEnabled: true,
		CallbackURL:     "https://voxis.test/api/v1/webhooks/speechmatics/s3cr3t",
	})

	blocks, ok := got["notification_config"].([]interface{})
	require.True(t, ok, "expected notification_config in %v", got)
	require.Len(t, blocks, 1)

	block, ok := blocks[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://voxis.test/api/v1/webhooks/speechmatics/s3cr3t", block["url"])
	assert.Equal(t, []interface{}{}, block["contents"])
	assert.NotContains(t, block, "auth_headers", "the secret travels in the URL path")
}

func TestSubmit_OmitsNotificationConfigWhenCallbackDisabled(t *testing.T) {
	// Disabled outright.
	got := submitAndCaptureWholeConfig(t, port.TranscriptionRequest{})
	assert.NotContains(t, got, "notification_config")

	// Enabled but with no URL to post to: still nothing to emit.
	got = submitAndCaptureWholeConfig(t, port.TranscriptionRequest{CallbackEnabled: true})
	assert.NotContains(t, got, "notification_config")

	// A URL without the opt-in flag must not leak into the job either.
	got = submitAndCaptureWholeConfig(t, port.TranscriptionRequest{
		CallbackURL: "https://voxis.test/api/v1/webhooks/speechmatics/s3cr3t",
	})
	assert.NotContains(t, got, "notification_config")
}
