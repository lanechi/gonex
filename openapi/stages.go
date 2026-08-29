package openapi

import (
	"reflect"

	"github.com/lanechi/gonex/router"
)

// routeDocumentationStage keeps route-level OpenAPI concerns separate from
// document assembly. The stage is intentionally internal so Generate remains
// the stable package entry point.
type routeDocumentationStage struct{}

func (routeDocumentationStage) Parameters(route router.Definition) []map[string]any {
	return parametersForRoute(route)
}

func (routeDocumentationStage) RequestBody(route router.Definition) map[string]any {
	return requestBodyForRoute(route)
}

func (routeDocumentationStage) Response(responseType reflect.Type, produces []string) map[string]any {
	return responseSchema(responseType, produces)
}

func (routeDocumentationStage) Security(values []string) []map[string][]string {
	return securityRequirements(values)
}

func (routeDocumentationStage) Schemas(components map[string]any, values ...reflect.Type) {
	for _, value := range values {
		addSchemaComponents(components, value)
	}
}
