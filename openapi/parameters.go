package openapi

import (
	"reflect"

	"github.com/lanechi/gonex/router"
)

// parametersForRoute renders non-body bindings as OpenAPI parameters.
func parametersForRoute(metadata router.RouteMetadata) []map[string]any {
	if metadata.RequestType == nil {
		return nil
	}
	requestType := indirectType(metadata.RequestType)
	if requestType == nil || requestType.Kind() != reflect.Struct {
		return nil
	}
	parameters := make([]map[string]any, 0)
	for _, field := range metadata.Bindings {
		fieldType := fieldTypeAt(requestType, field.Index)
		fieldStruct := fieldStructAt(requestType, field.Index)
		required := fieldIsRequired(fieldStruct)
		for _, parameter := range []struct{ source, name string }{{"path", field.Path}, {"query", field.Query}, {"header", field.Header}, {"cookie", field.Cookie}} {
			if parameter.name == "" {
				continue
			}
			parameters = append(parameters, map[string]any{"name": parameter.name, "in": parameter.source, "required": required || parameter.source == "path", "schema": schemaForField(fieldStruct, fieldType)})
		}
	}
	return parameters
}
