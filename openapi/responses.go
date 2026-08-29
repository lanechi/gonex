package openapi

import (
	"reflect"
)

// responseSchema builds the standard envelope schema for a controller result.
func responseSchema(responseType reflect.Type, produces []string) map[string]any {
	dataSchema := map[string]any{"nullable": true}
	if responseType != nil {
		dataSchema = schemaForType(responseType)
	}
	mediaTypes := produces
	if len(mediaTypes) == 0 {
		mediaTypes = []string{"application/json"}
	}
	content := make(map[string]any, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		content[mediaType] = map[string]any{"schema": map[string]any{
			"type": "object", "properties": map[string]any{
				"code": map[string]any{"type": "integer"}, "message": map[string]any{"type": "string"}, "data": dataSchema,
			},
		}}
	}
	return map[string]any{"description": "OK", "content": content}
}
