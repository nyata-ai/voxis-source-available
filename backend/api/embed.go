// Package api embeds the Voxis Source-Available OpenAPI contract.
package api

import _ "embed"

// OpenAPISpec is the public-safe OSS specification.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
