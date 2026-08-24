// Package openapi contains the OpenAPI document model and normalization
// helpers used by the framework generator.
package openapi

import "strings"

type Document struct {
	OpenAPI    string                          `json:"openapi"`
	Info       Info                            `json:"info"`
	Paths      map[string]map[string]Operation `json:"paths"`
	Components map[string]any                  `json:"components,omitempty"`
}

type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
	Parameters  []map[string]any      `json:"parameters,omitempty"`
	RequestBody map[string]any        `json:"requestBody,omitempty"`
	Responses   map[string]any        `json:"responses"`
}

// Path converts Gin-style :id and *filepath segments to OpenAPI parameters.
func Path(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if len(part) > 1 && (part[0] == ':' || part[0] == '*') {
			parts[index] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
