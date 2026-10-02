package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ServeOpenAPI returns a Gin handler that serves the OpenAPI specification as
// JSON. It parses the YAML spec once, injects publicBaseURL into
// servers[0].url, marshals to JSON, and caches the result for all subsequent
// requests.
func ServeOpenAPI(specYAML []byte, publicBaseURL string) gin.HandlerFunc {
	// Parse YAML into a generic map.
	var spec map[string]interface{}
	if err := yaml.Unmarshal(specYAML, &spec); err != nil {
		// If the spec is invalid, return 500 on every request.
		return func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "failed to parse OpenAPI spec",
			})
		}
	}

	// Inject the public base URL into servers[0].url.
	if servers, ok := spec["servers"].([]interface{}); ok && len(servers) > 0 {
		if server0, ok := servers[0].(map[string]interface{}); ok {
			server0["url"] = publicBaseURL
		}
	}

	// Marshal to JSON once (cached for all requests).
	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		return func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "failed to marshal OpenAPI spec to JSON",
			})
		}
	}

	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", jsonBytes)
	}
}
