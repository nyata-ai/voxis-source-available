package api

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIContainsOnlyRetainedPaths(t *testing.T) {
	var spec struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(OpenAPISpec, &spec); err != nil {
		t.Fatalf("parse OSS OpenAPI: %v", err)
	}
	for _, path := range []string{"/api/v1/media/upload", "/api/v1/recordings", "/api/v1/transcriptions", "/api/v1/summaries", "/api/v1/admin/ops/stats", "/mcp"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("missing retained path %s", path)
		}
	}
	for _, path := range []string{"/api/v1/transcriptions/url", "/api/v1/billing", "/api/v1/payments", "/api/v1/privilege-recordings"} {
		if _, ok := spec.Paths[path]; ok {
			t.Fatalf("excluded path present: %s", path)
		}
	}
}

func TestOpenAPISemanticPathParameterValidation(t *testing.T) {
	var spec struct {
		OpenAPI    string                    `yaml:"openapi"`
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Parameters map[string]map[string]any `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(OpenAPISpec, &spec); err != nil {
		t.Fatalf("parse OSS OpenAPI: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI version = %q, want 3.1.0", spec.OpenAPI)
	}
	for path, item := range spec.Paths {
		declared := make(map[string]bool)
		collectPathParameters(t, declared, item["parameters"], spec.Components.Parameters)
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := item[method].(map[string]any)
			if ok {
				collectPathParameters(t, declared, operation["parameters"], spec.Components.Parameters)
			}
		}
		for _, name := range pathTemplateParameters(path) {
			if !declared[name] {
				t.Errorf("%s is missing required path parameter %q", path, name)
			}
		}
	}
}

func collectPathParameters(t *testing.T, declared map[string]bool, raw any, components map[string]map[string]any) {
	t.Helper()
	parameters, ok := raw.([]any)
	if !ok {
		return
	}
	for _, rawParameter := range parameters {
		parameter, ok := rawParameter.(map[string]any)
		if !ok {
			t.Errorf("path parameter has invalid type %T", rawParameter)
			continue
		}
		if ref, ok := parameter["$ref"].(string); ok {
			const prefix = "#/components/parameters/"
			if !strings.HasPrefix(ref, prefix) {
				t.Errorf("unsupported parameter reference %q", ref)
				continue
			}
			var found bool
			parameter, found = components[strings.TrimPrefix(ref, prefix)]
			if !found {
				t.Errorf("missing parameter component %q", ref)
				continue
			}
		}
		name, _ := parameter["name"].(string)
		location, _ := parameter["in"].(string)
		if location != "path" {
			continue
		}
		required, _ := parameter["required"].(bool)
		if name == "" || !required {
			t.Errorf("invalid path parameter %s", fmt.Sprint(parameter))
			continue
		}
		declared[name] = true
	}
}

func pathTemplateParameters(path string) []string {
	parameters := make([]string, 0, 2)
	for rest := path; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			return parameters
		}
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return parameters
		}
		parameters = append(parameters, rest[:end])
		rest = rest[end+1:]
	}
}
