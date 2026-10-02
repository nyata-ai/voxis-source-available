package speechmatics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/voxis/backend/internal/domain"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "sm-key")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, DefaultAPIURL, cfg.APIURL)
	assert.Equal(t, ModeSaaS, cfg.Mode)
	assert.Equal(t, "sm-key", cfg.APIKey)
	assert.Nil(t, cfg.LanguageHints)
}

func TestLoadConfig_TrimsTrailingSlash(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_API_URL", "https://eu1.asr.api.speechmatics.com/")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://eu1.asr.api.speechmatics.com", cfg.APIURL)
}

func TestLoadConfig_RequiresKeyInSaaSMode(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_ContainerModeAllowsHTTPAndNoKey(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "")
	t.Setenv("SPEECHMATICS_MODE", "container")
	t.Setenv("SPEECHMATICS_API_URL", "http://speechmatics.internal:8080")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, ModeContainer, cfg.Mode)
}

func TestLoadConfig_RejectsPlaintextSaaSURL(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_API_URL", "http://eu1.asr.api.speechmatics.com")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_RejectsUnknownMode(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_MODE", "hybrid")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_MapsAndValidatesLanguageHints(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_LANGUAGE_HINTS", "id, zh , en")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"cmn", "en", "id"}, cfg.LanguageHints)
}

func TestLoadConfig_DiarizationTuningDefaultsToNil(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg.SpeakerSensitivity)
	assert.Nil(t, cfg.PreferCurrentSpeaker)
}

func TestLoadConfig_ParsesSpeakerSensitivity(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_SPEAKER_SENSITIVITY", "0.3")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.SpeakerSensitivity)
	assert.InDelta(t, 0.3, *cfg.SpeakerSensitivity, 0.0001)
}

func TestLoadConfig_SpeakerSensitivityBoundsAreInclusive(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")

	for _, v := range []string{"0", "1", "0.0", "1.0"} {
		t.Setenv("SPEECHMATICS_SPEAKER_SENSITIVITY", v)
		cfg, err := LoadConfig()
		require.NoError(t, err, "value %q should be accepted", v)
		require.NotNil(t, cfg.SpeakerSensitivity)
	}
}

func TestLoadConfig_RejectsOutOfRangeSpeakerSensitivity(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")

	for _, v := range []string{"1.5", "-0.1"} {
		t.Setenv("SPEECHMATICS_SPEAKER_SENSITIVITY", v)
		_, err := LoadConfig()
		require.Error(t, err, "value %q should be rejected", v)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	}
}

func TestLoadConfig_RejectsNonNumericSpeakerSensitivity(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_SPEAKER_SENSITIVITY", "low")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_BlankSpeakerSensitivityMeansUnset(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_SPEAKER_SENSITIVITY", "   ")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg.SpeakerSensitivity)
}

func TestLoadConfig_ParsesPreferCurrentSpeaker(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")

	t.Setenv("SPEECHMATICS_PREFER_CURRENT_SPEAKER", "true")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.PreferCurrentSpeaker)
	assert.True(t, *cfg.PreferCurrentSpeaker)

	t.Setenv("SPEECHMATICS_PREFER_CURRENT_SPEAKER", "false")
	cfg, err = LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg.PreferCurrentSpeaker)
	assert.False(t, *cfg.PreferCurrentSpeaker)
}

func TestLoadConfig_RejectsInvalidPreferCurrentSpeaker(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_PREFER_CURRENT_SPEAKER", "yes")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_BlankPreferCurrentSpeakerMeansUnset(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_PREFER_CURRENT_SPEAKER", "   ")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg.PreferCurrentSpeaker)
}

func TestLoadConfig_WebhookEnabledDefaultsOff(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_WEBHOOK_ENABLED", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.WebhookEnabled)
}

func TestLoadConfig_ParsesWebhookEnabled(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")

	t.Setenv("SPEECHMATICS_WEBHOOK_ENABLED", "true")
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.True(t, cfg.WebhookEnabled)

	t.Setenv("SPEECHMATICS_WEBHOOK_ENABLED", "false")
	cfg, err = LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.WebhookEnabled)
}

func TestLoadConfig_RejectsInvalidWebhookEnabled(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_WEBHOOK_ENABLED", "sometimes")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestLoadConfig_RejectsUnknownLanguageHint(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "k")
	t.Setenv("SPEECHMATICS_LANGUAGE_HINTS", "id,klingon")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestMapLanguageHints(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{"empty", nil, nil, false},
		{"auto is not a hint", []string{"auto"}, nil, false},
		{"zh becomes cmn", []string{"zh"}, []string{"cmn"}, false},
		{"sorted and deduped", []string{"id", "en", "id"}, []string{"en", "id"}, false},
		{"jv and su have no melia hint and are dropped", []string{"jv", "su", "tl", "ms"}, []string{"ms", "tl"}, false},
		{"only unsupported codes yields no hints and no error", []string{"jv", "su", "af", "la"}, nil, false},
		{"case insensitive", []string{"ID", " En "}, []string{"en", "id"}, false},
		{"unknown code fails loudly", []string{"id", "xx"}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MapLanguageHints(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, domain.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Every user-selectable language must be a known code to MapLanguageHints, or
// a picker choice would fail at submit time instead of at startup. The four
// codes Melia has no language for (jv, su, af, la) are the one carved-out
// exception: they must return no error and contribute zero hints, never fail.
func TestMapLanguageHints_CoversEveryAllowedLanguage(t *testing.T) {
	meliaUnsupported := map[string]bool{"jv": true, "su": true, "af": true, "la": true}

	for code := range domain.AllowedLanguages {
		if code == "auto" {
			continue
		}
		got, err := MapLanguageHints([]string{code})
		require.NoErrorf(t, err, "language %q has no Speechmatics mapping", code)
		if meliaUnsupported[code] {
			assert.Emptyf(t, got, "language %q has no Melia hint and must yield zero hints", code)
			continue
		}
		require.Len(t, got, 1)
	}
}

func TestDroppedLanguageHints(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"none dropped", []string{"id", "en"}, nil},
		{"unsupported codes reported, sorted and deduped", []string{"la", "af", "jv", "af"}, []string{"af", "jv", "la"}},
		{"mixed supported and unsupported", []string{"jv", "id", "su"}, []string{"jv", "su"}},
		{"case insensitive", []string{"JV", " Su "}, []string{"jv", "su"}},
		{"an unknown code is not a drop", []string{"xx"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DroppedLanguageHints(tt.in))
		})
	}
}
