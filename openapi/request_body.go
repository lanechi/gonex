package openapi

import (
	"reflect"

	"github.com/lanechi/gonex/router"
)

func requestBodyForRoute(metadata router.RouteMetadata) map[string]any {
	if metadata.RequestType == nil {
		return nil
	}
	requestType := indirectType(metadata.RequestType)
	if requestType == nil || requestType.Kind() != reflect.Struct {
		return nil
	}
	jsonSchema, hasJSON := bodySchema(requestType, "json")
	formSchema, hasForm := bodySchema(requestType, "form")
	if len(metadata.Consumes) > 0 {
		hasJSON = hasJSON && containsString(metadata.Consumes, "application/json")
		hasForm = hasForm && (containsString(metadata.Consumes, "application/x-www-form-urlencoded") || containsString(metadata.Consumes, "multipart/form-data"))
	}
	if !hasJSON && !hasForm {
		return nil
	}
	content := make(map[string]any)
	required := false
	if hasJSON {
		content["application/json"] = map[string]any{"schema": jsonSchema}
		required = required || schemaRequiresBody(jsonSchema)
	}
	if hasForm {
		for _, mediaType := range formMediaTypes(metadata) {
			content[mediaType] = map[string]any{"schema": formSchema}
		}
		required = required || schemaRequiresBody(formSchema)
	}
	requestBody := map[string]any{"content": content}
	if required {
		requestBody["required"] = true
	}
	return requestBody
}

func formMediaTypes(metadata router.RouteMetadata) []string {
	if len(metadata.Consumes) == 0 {
		if routeUsesMultipart(metadata) {
			return []string{"multipart/form-data"}
		}
		return []string{"application/x-www-form-urlencoded"}
	}
	mediaTypes := make([]string, 0, 2)
	for _, mediaType := range []string{"application/x-www-form-urlencoded", "multipart/form-data"} {
		if containsString(metadata.Consumes, mediaType) {
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	return mediaTypes
}

func routeUsesMultipart(metadata router.RouteMetadata) bool {
	if metadata.RequestType == nil {
		return false
	}
	requestType := indirectType(metadata.RequestType)
	for _, binding := range metadata.Bindings {
		if binding.File != "" {
			return true
		}
		if binding.Form == "" {
			continue
		}
		fieldType := fieldTypeAt(requestType, binding.Index)
		if fieldType == multipartFileHeaderType || fieldType.Kind() == reflect.Slice && fieldType.Elem() == multipartFileHeaderType {
			return true
		}
	}
	return false
}

func schemaRequiresBody(schema map[string]any) bool {
	required, ok := schema["required"].([]string)
	return ok && len(required) > 0
}

func bodySchema(structType reflect.Type, source string) (map[string]any, bool) {
	properties := make(map[string]any)
	required := make([]string, 0)
	found := false
	for fieldIndex := 0; fieldIndex < structType.NumField(); fieldIndex++ {
		field := structType.Field(fieldIndex)
		if field.Anonymous && isMetaField(field.Type) || field.PkgPath != "" && !field.Anonymous {
			continue
		}
		if field.Anonymous && isEmbeddedStruct(field.Type) && !hasBindingTags(field) {
			nested, nestedFound := bodySchema(indirectType(field.Type), source)
			if nestedFound {
				found = true
				required = mergeSchemaProperties(properties, required, nested)
			}
			continue
		}
		if hasAnyParameterTag(field) && !(source == "form" && (fieldTagExists(field, "form") || fieldTagExists(field, "file"))) {
			continue
		}
		if source == "json" {
			name, _ := fieldTagName(field, "json", field.Name)
			if name == "" {
				continue
			}
			properties[name] = schemaForField(field, field.Type)
			found = true
			if fieldIsRequired(field) {
				required = append(required, name)
			}
		} else {
			name, ok := fieldTagName(field, "form", "")
			if !ok {
				name, ok = fieldTagName(field, "file", "")
			}
			if !ok || name == "" {
				continue
			}
			properties[name] = schemaForField(field, field.Type)
			found = true
			if fieldIsRequired(field) {
				required = append(required, name)
			}
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, found
}

func mergeSchemaProperties(properties map[string]any, required []string, nested map[string]any) []string {
	if nestedProperties, ok := nested["properties"].(map[string]any); ok {
		for name, value := range nestedProperties {
			properties[name] = value
		}
	}
	if nestedRequired, ok := nested["required"].([]string); ok {
		required = append(required, nestedRequired...)
	}
	return required
}
