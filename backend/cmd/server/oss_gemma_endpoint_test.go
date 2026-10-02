package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func fixedLookup(ips ...string) hostLookup {
	return func(context.Context, string) ([]net.IPAddr, error) {
		addrs := make([]net.IPAddr, 0, len(ips))
		for _, ip := range ips {
			addrs = append(addrs, net.IPAddr{IP: net.ParseIP(ip)})
		}
		return addrs, nil
	}
}

func failingLookup(context.Context, string) ([]net.IPAddr, error) {
	return nil, errors.New("no such host")
}

func TestGemmaEndpointIsPublic(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		lookup hostLookup
		public bool
	}{
		{name: "compose service", url: "http://llama:8080/v1", lookup: failingLookup, public: false},
		{name: "localhost", url: "http://localhost:8080/v1", lookup: failingLookup, public: false},
		{name: "loopback", url: "http://127.0.0.1:8080/v1", lookup: failingLookup, public: false},
		{name: "rfc1918", url: "http://10.0.0.5/v1", lookup: failingLookup, public: false},
		{name: "ipv6 ula", url: "http://[fd00::5]:8080/v1", lookup: failingLookup, public: false},
		{name: "public ip", url: "https://8.8.8.8/v1", lookup: failingLookup, public: true},
		{name: "name resolving private", url: "https://gemma.internal.example/v1", lookup: fixedLookup("10.1.2.3"), public: false},
		{name: "name resolving public", url: "https://api.example.com/v1", lookup: fixedLookup("10.1.2.3", "93.184.216.34"), public: true},
		{name: "unresolvable name", url: "https://gemma.example.com/v1", lookup: failingLookup, public: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			public, _ := gemmaEndpointIsPublic(context.Background(), test.url, test.lookup)
			if public != test.public {
				t.Fatalf("gemmaEndpointIsPublic(%q) = %t, want %t", test.url, public, test.public)
			}
		})
	}
}

func TestWarnIfGemmaEndpointPublicLogsWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	warnIfGemmaEndpointPublic(context.Background(), logger, "https://8.8.8.8/v1", failingLookup)
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Fatalf("log = %q, want a warning", buf.String())
	}
	buf.Reset()
	warnIfGemmaEndpointPublic(context.Background(), logger, "http://llama:8080/v1", failingLookup)
	if buf.Len() != 0 {
		t.Fatalf("log = %q, want no warning for a compose service", buf.String())
	}
}
