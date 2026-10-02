package domain

import "testing"

func TestOSSNormalizeAudioContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{contentType: "audio/vnd.dlna.adts", want: "audio/aac"},
		{contentType: "audio/x-aac", want: "audio/aac"},
		{contentType: "audio/wave", want: "audio/wav"},
		{contentType: "audio/vnd.dlna.adts; codecs=mp4a.40.2", want: "audio/aac"},
		{contentType: "application/pdf", want: "application/pdf"},
	}

	for _, tt := range tests {
		if got := NormalizeAudioContentType(tt.contentType); got != tt.want {
			t.Errorf("NormalizeAudioContentType(%q) = %q, want %q", tt.contentType, got, tt.want)
		}
	}
}
