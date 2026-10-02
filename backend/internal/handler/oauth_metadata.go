package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OAuthMetadataConfig is the static configuration used to render the
// OAuth 2.0 Protected Resource Metadata document (RFC 9728) served at
// /.well-known/oauth-protected-resource. The MCP spec (2025-06-18 §2.3)
// requires MCP servers to expose this document so remote MCP clients
// (Claude.ai, ChatGPT, custom wrappers) can discover the authorization
// server without manual configuration.
type OAuthMetadataConfig struct {
	// ResourceURL is the canonical URI of the protected resource (the MCP
	// endpoint, e.g. https://voxis.example.com/mcp). MCP clients use this
	// value as the `resource` parameter (RFC 8707) when requesting tokens.
	ResourceURL string
	// AuthServerURLs is the list of authorization server issuer identifiers
	// (e.g. the Keycloak realm URL). MCP clients fetch /.well-known/oauth-
	// authorization-server from these to learn endpoints.
	AuthServerURLs []string
	// ScopesSupported is the list of OAuth scopes a client may request for
	// this resource. Mirrors the API key scope vocabulary.
	ScopesSupported []string
	// ResourceName is a human-readable label shown in some MCP client UIs.
	ResourceName string
	// DocumentationURL is an optional developer documentation URL.
	DocumentationURL string
}

// oauthMetadataResponse mirrors the fields defined in RFC 9728 §2.
// JSON tags follow the spec exactly.
type oauthMetadataResponse struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
	ResourceName           string   `json:"resource_name,omitempty"`
	ResourceDocumentation  string   `json:"resource_documentation,omitempty"`
}

// OAuthProtectedResourceMetadata serves the RFC 9728 Protected Resource
// Metadata document. Mount at GET /.well-known/oauth-protected-resource
// with no auth middleware — the document itself is public per RFC 9728 §3.
func OAuthProtectedResourceMetadata(cfg OAuthMetadataConfig) gin.HandlerFunc {
	resp := oauthMetadataResponse{
		Resource:               cfg.ResourceURL,
		AuthorizationServers:   cfg.AuthServerURLs,
		ScopesSupported:        cfg.ScopesSupported,
		BearerMethodsSupported: []string{"header"},
		ResourceName:           cfg.ResourceName,
		ResourceDocumentation:  cfg.DocumentationURL,
	}
	return func(c *gin.Context) {
		// The metadata is static per process; allow brief caching to reduce
		// load when many MCP clients bootstrap concurrently. SecurityHeaders
		// already sets Cache-Control: no-store globally — override here so
		// clients can cache for 5 minutes (RFC 9728 doesn't specify, but
		// short-cache matches OAuth AS metadata conventions).
		c.Header("Cache-Control", "public, max-age=300")
		// Browser-based MCP clients (Claude.ai, ChatGPT) fetch this document
		// cross-origin. The global CORS middleware only allows pre-configured
		// origins; the discovery document is RFC 9728 public-by-design, so we
		// stamp permissive CORS here. No credentials are required, so `*` is
		// safe and matches RFC 6454 best practice for public resources.
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Vary", "Origin")
		c.JSON(http.StatusOK, resp)
	}
}

// MCPProtectedResourceMetadataURL returns the canonical URL where the
// Protected Resource Metadata document is served, given the public base URL
// of the API. Returns empty string when publicBaseURL is empty, which lets
// callers conditionally skip header stamping during early startup.
func MCPProtectedResourceMetadataURL(publicBaseURL string) string {
	if publicBaseURL == "" {
		return ""
	}
	return publicBaseURL + "/.well-known/oauth-protected-resource"
}
