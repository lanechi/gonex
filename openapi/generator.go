package openapi

import (
	"reflect"
	"strings"

	"github.com/lanechi/gonex/router"
)

// Generate builds an OpenAPI 3 document from framework-owned route definitions.
func Generate(name string, routes []router.Definition) Document {
	document := Document{OpenAPI: "3.0.3", Info: Info{Title: name, Version: "0.1.0"}, Paths: map[string]map[string]Operation{}, Components: map[string]any{"schemas": map[string]any{}, "securitySchemes": map[string]any{}}}
	for _, route := range routes {
		metadata := route.Metadata
		op := Operation{Tags: metadata.Tags, Summary: metadata.Summary, Description: metadata.Description, OperationID: metadata.OperationID, Deprecated: metadata.Deprecated, Security: securityRequirements(metadata.Security), Parameters: parametersForRoute(route), Responses: map[string]any{"200": responseSchema(metadata.ResponseType, metadata.Produces), "400": map[string]any{"description": "Bad Request"}, "500": map[string]any{"description": "Internal Server Error"}}}
		op.RequestBody = requestBodyForRoute(route)
		path := Path(metadata.Path)
		method := strings.ToLower(metadata.Method)
		if document.Paths[path] == nil {
			document.Paths[path] = map[string]Operation{}
		}
		document.Paths[path][method] = op
		addSecuritySchemes(document.Components, metadata.Security)
		addSchemaComponents(document.Components, metadata.RequestType)
		addSchemaComponents(document.Components, metadata.ResponseType)
	}
	return document
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}
func fieldTypeAt(t reflect.Type, index []int) reflect.Type {
	for _, part := range index {
		t = indirectType(t).Field(part).Type
	}
	return t
}
func fieldStructAt(t reflect.Type, index []int) reflect.StructField {
	var f reflect.StructField
	for _, part := range index {
		f = indirectType(t).Field(part)
		t = f.Type
	}
	return f
}
func hasAnyParameterTag(f reflect.StructField) bool {
	for _, tag := range []string{"path", "query", "header", "cookie", "form", "file"} {
		if fieldTagExists(f, tag) {
			return true
		}
	}
	return false
}
