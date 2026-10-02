package main

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"
)

const gemmaEndpointLookupTimeout = 2 * time.Second

type hostLookup func(ctx context.Context, host string) ([]net.IPAddr, error)

// warnIfGemmaEndpointPublic logs a startup warning when GEMMA_BASE_URL points
// at a public address. The documentation promises that summaries and answers
// are generated locally; transcripts sent to a public host break that promise.
func warnIfGemmaEndpointPublic(ctx context.Context, logger *slog.Logger, baseURL string, lookup hostLookup) {
	public, reason := gemmaEndpointIsPublic(ctx, baseURL, lookup)
	if !public {
		return
	}
	logger.Warn("GEMMA_BASE_URL is not a loopback, private, or single-label service address; transcripts sent for summaries leave the local network",
		"reason", reason)
}

// gemmaEndpointIsPublic reports whether the endpoint host may be public.
// Loopback, RFC 1918, IPv6 ULA, link-local, "localhost", and single-label
// (compose service) names count as local. A dotted name is local only when
// every address it resolves to is.
func gemmaEndpointIsPublic(ctx context.Context, baseURL string, lookup hostLookup) (public bool, reason string) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return true, "endpoint URL has no host"
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if ip := net.ParseIP(host); ip != nil {
		return !isLocalAddress(ip), "address is public"
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return false, ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, gemmaEndpointLookupTimeout)
	defer cancel()
	addrs, err := lookup(lookupCtx, host)
	if err != nil || len(addrs) == 0 {
		return true, "host name could not be resolved to verify it is local"
	}
	for _, addr := range addrs {
		if !isLocalAddress(addr.IP) {
			return true, "host name resolves to a public address"
		}
	}
	return false, ""
}

func isLocalAddress(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
