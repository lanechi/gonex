package openapi

import (
	"encoding/json"
	"mime/multipart"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/lanechi/gonex/router"
)

var (
	multipartFileHeaderType = reflect.TypeOf((*multipart.FileHeader)(nil))
)

// Generate builds an OpenAPI 3 document from framework-owned route definitions.
// The generator depends on router definitions rather than Server, so tooling
// can generate documents without constructing an HTTP server.
func Generate(name string, routes []router.Definition) Document {
	document := Document{
		OpenAPI: "3.0.3",
		Info: Info{
			Title:   name,
			Version: "0.1.0",
		},
		Paths: make(map[string]map[string]Operation),
		Components: map[string]any{
			"schemas":         map[string]any{},
			"securitySchemes": map[string]any{},
		},
	}

	for _, route := range routes {
		operation := Operation{
			Tags:        route.Tags,
			Summary:     route.Summary,
			Description: route.Description,
			OperationID: route.OperationID,
			Deprecated:  route.Deprecated,
			Security:    securityRequirements(route.Security),
			Parameters:  parametersForRoute(route),
			Responses: map[string]any{
				"200": responseSchema(route.ResponseType, route.Produces),
				"400": map[string]any{"description": "Bad Request"},
				"500": map[string]any{"description": "Internal Server Error"},
			},
		}
		operation.RequestBody = requestBodyForRoute(route)
		path := Path(route.Path)
		method := strings.ToLower(route.Method)
		if document.Paths[path] == nil {
			document.Paths[path] = make(map[string]Operation)
		}
		document.Paths[path][method] = operation
		addSecuritySchemes(document.Components, route.Security)
		addSchemaComponents(document.Components, route.RequestType)
		addSchemaComponents(document.Components, route.ResponseType)
	}
	return document
}

// Clone returns a deep copy suitable for returning cached documents safely.
func Clone(document Document) Document {
	clone := Document{
		OpenAPI: document.OpenAPI,
		Info:    document.Info,
		Paths:   make(map[string]map[string]Operation, len(document.Paths)),
	}
	for path, methods := range document.Paths {
		clone.Paths[path] = make(map[string]Operation, len(methods))
		for method, operation := range methods {
			operation.Tags = append([]string(nil), operation.Tags...)
			operation.Security = append([]map[string][]string(nil), operation.Security...)
			for index := range operation.Security {
				security := make(map[string][]string, len(operation.Security[index]))
				for name, scopes := range operation.Security[index] {
					security[name] = append([]string(nil), scopes...)
				}
				operation.Security[index] = security
			}
			operation.Parameters = cloneMapSlice(operation.Parameters)
			operation.RequestBody = cloneMap(operation.RequestBody)
			operation.Responses = cloneMap(operation.Responses)
			clone.Paths[path][method] = operation
		}
	}
	if document.Components != nil {
		clone.Components = cloneMap(document.Components)
	}
	return clone
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneValue(item)
	}
	return clone
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	clone := make([]map[string]any, len(values))
	for index, value := range values {
		clone[index] = cloneMap(value)
	}
	return clone
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneMap(value)
	case []any:
		clone := make([]any, len(value))
		for index, item := range value {
			clone[index] = cloneValue(item)
		}
		return clone
	case []string:
		return append([]string(nil), value...)
	case []map[string]any:
		return cloneMapSlice(value)
	default:
		return value
	}
}

func parametersForRoute(route router.Definition) []map[string]any {
	if route.Binder == nil || route.RequestType == nil {
		return nil
	}
	requestType := indirectType(route.RequestType)
	if requestType == nil || requestType.Kind() != reflect.Struct {
		return nil
	}
	parameters := make([]map[string]any, 0)
	for _, field := range route.Binder.Fields {
		fieldType := fieldTypeAt(requestType, field.Index)
		fieldStruct := fieldStructAt(requestType, field.Index)
		required := fieldIsRequired(fieldStruct)
		for _, parameter := range []struct {
			source string
			name   string
		}{
			{source: "path", name: field.Path},
			{source: "query", name: field.Query},
			{source: "header", name: field.Header},
			{source: "cookie", name: field.Cookie},
		} {
			if parameter.name == "" {
				continue
			}
			parameters = append(parameters, map[string]any{
				"name":     parameter.name,
				"in":       parameter.source,
				"required": required || parameter.source == "path",
				"schema":   schemaForField(fieldStruct, fieldType),
			})
		}
	}
	return parameters
}

func requestBodyForRoute(route router.Definition) map[string]any {
	if route.RequestType == nil {
		return nil
	}
	requestType := indirectType(route.RequestType)
	if requestType == nil || requestType.Kind() != reflect.Struct {
		return nil
	}
	jsonSchema, hasJSON := bodySchema(requestType, "json")
	formSchema, hasForm := bodySchema(requestType, "form")
	if len(route.Consumes) > 0 {
		hasJSON = hasJSON && containsString(route.Consumes, "application/json")
		hasForm = hasForm && (containsString(route.Consumes, "application/x-www-form-urlencoded") || containsString(route.Consumes, "multipart/form-data"))
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
		for _, mediaType := range formMediaTypes(route) {
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

func formMediaTypes(route router.Definition) []string {
	if len(route.Consumes) == 0 {
		if routeUsesMultipart(route) {
			return []string{"multipart/form-data"}
		}
		return []string{"application/x-www-form-urlencoded"}
	}
	mediaTypes := make([]string, 0, 2)
	for _, mediaType := range []string{"application/x-www-form-urlencoded", "multipart/form-data"} {
		if containsString(route.Consumes, mediaType) {
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	return mediaTypes
}

func routeUsesMultipart(route router.Definition) bool {
	if route.Binder == nil || route.RequestType == nil {
		return false
	}
	requestType := indirectType(route.RequestType)
	for _, binding := range route.Binder.Fields {
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
		if field.Anonymous && isMetaField(field.Type) {
			continue
		}
		if field.PkgPath != "" && !field.Anonymous {
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
		} else if source == "form" {
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
			"type": "object",
			"properties": map[string]any{
				"code":    map[string]any{"type": "integer"},
				"message": map[string]any{"type": "string"},
				"data":    dataSchema,
			},
		}}
	}
	return map[string]any{"description": "OK", "content": content}
}

func schemaForType(fieldType reflect.Type) map[string]any {
	return schemaForTypeWithSeen(fieldType, make(map[reflect.Type]bool))
}

func schemaForTypeWithSeen(fieldType reflect.Type, seen map[reflect.Type]bool) map[string]any {
	if fieldType == nil {
		return map[string]any{}
	}
	if fieldType == multipartFileHeaderType {
		return map[string]any{"type": "string", "format": "binary"}
	}
	if fieldType.Kind() == reflect.Slice && fieldType.Elem() == multipartFileHeaderType {
		return map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string", "format": "binary"},
		}
	}
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	if seen[fieldType] {
		return map[string]any{"type": "object"}
	}
	seen[fieldType] = true
	defer delete(seen, fieldType)
	if fieldType == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	switch fieldType.Kind() {
	case reflect.Struct:
		properties := make(map[string]any)
		required := make([]string, 0)
		for fieldIndex := 0; fieldIndex < fieldType.NumField(); fieldIndex++ {
			field := fieldType.Field(fieldIndex)
			if field.Anonymous && isMetaField(field.Type) {
				continue
			}
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}
			if field.Anonymous && isEmbeddedStruct(field.Type) && !fieldTagExists(field, "json") {
				nested := schemaForTypeWithSeen(field.Type, seen)
				required = mergeSchemaProperties(properties, required, nested)
				continue
			}
			name, _ := fieldTagName(field, "json", field.Name)
			if name == "" {
				continue
			}
			properties[name] = schemaForFieldWithSeen(field, field.Type, seen)
			if fieldIsRequired(field) {
				required = append(required, name)
			}
		}
		schema := map[string]any{"type": "object", "properties": properties}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	case reflect.Slice, reflect.Array:
		if fieldType.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": schemaForTypeWithSeen(fieldType.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaForTypeWithSeen(fieldType.Elem(), seen)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer", "format": integerFormat(fieldType.Bits())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer", "format": integerFormat(fieldType.Bits()), "minimum": 0}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number", "format": floatFormat(fieldType.Bits())}
	default:
		return map[string]any{}
	}
}

func schemaForField(field reflect.StructField, fieldType reflect.Type) map[string]any {
	return schemaForFieldWithSeen(field, fieldType, make(map[reflect.Type]bool))
}

func schemaForFieldWithSeen(field reflect.StructField, fieldType reflect.Type, seen map[reflect.Type]bool) map[string]any {
	schema := schemaForTypeWithSeen(fieldType, seen)
	if description := firstNonEmptyTag(field, "description", "dc"); description != "" {
		schema["description"] = description
	}
	for _, tagName := range []string{"example", "default"} {
		if raw := strings.TrimSpace(field.Tag.Get(tagName)); raw != "" {
			schema[tagName] = parseJSONTagValue(raw)
		}
	}
	if raw := strings.TrimSpace(field.Tag.Get("enum")); raw != "" {
		values := make([]any, 0)
		for _, value := range splitTag(raw) {
			values = append(values, parseTypedTagValue(value, fieldType))
		}
		if len(values) > 0 {
			schema["enum"] = values
		}
	}
	for _, tagName := range []string{"binding", "validate"} {
		for _, rule := range strings.Split(field.Tag.Get(tagName), ",") {
			name, value, hasValue := strings.Cut(strings.TrimSpace(rule), "=")
			if !hasValue || value == "" {
				continue
			}
			switch name {
			case "gte", "min":
				schema[constraintKey(fieldType, "minimum")] = parseJSONTagValue(value)
			case "gt":
				applyExclusiveConstraint(schema, fieldType, value, true)
			case "lte", "max":
				schema[constraintKey(fieldType, "maximum")] = parseJSONTagValue(value)
			case "lt":
				applyExclusiveConstraint(schema, fieldType, value, false)
			}
		}
	}
	return schema
}

func applyExclusiveConstraint(schema map[string]any, fieldType reflect.Type, raw string, minimum bool) {
	fieldType = indirectType(fieldType)
	switch fieldType.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return
		}
		if minimum {
			value++
			schema[constraintKey(fieldType, "minimum")] = value
		} else {
			value--
			if value >= 0 {
				schema[constraintKey(fieldType, "maximum")] = value
			}
		}
	default:
		value := parseJSONTagValue(raw)
		if minimum {
			schema["minimum"] = value
			schema["exclusiveMinimum"] = true
		} else {
			schema["maximum"] = value
			schema["exclusiveMaximum"] = true
		}
	}
}

func constraintKey(fieldType reflect.Type, numericKey string) string {
	fieldType = indirectType(fieldType)
	isMinimum := strings.Contains(strings.ToLower(numericKey), "minimum")
	switch fieldType.Kind() {
	case reflect.String:
		if isMinimum {
			return "minLength"
		}
		return "maxLength"
	case reflect.Slice, reflect.Array:
		if isMinimum {
			return "minItems"
		}
		return "maxItems"
	case reflect.Map:
		if isMinimum {
			return "minProperties"
		}
		return "maxProperties"
	default:
		return numericKey
	}
}

func firstNonEmptyTag(field reflect.StructField, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(field.Tag.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func parseJSONTagValue(raw string) any {
	var value any
	if json.Unmarshal([]byte(raw), &value) == nil {
		return value
	}
	if parsed, err := strconv.ParseBool(raw); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
		return parsed
	}
	return raw
}

func parseTypedTagValue(raw string, fieldType reflect.Type) any {
	fieldType = indirectType(fieldType)
	switch fieldType.Kind() {
	case reflect.Bool:
		if value, err := strconv.ParseBool(raw); err == nil {
			return value
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return value
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value, err := strconv.ParseUint(raw, 10, 64); err == nil {
			return value
		}
	case reflect.Float32, reflect.Float64:
		if value, err := strconv.ParseFloat(raw, 64); err == nil {
			return value
		}
	}
	return raw
}

func securityRequirements(security []string) []map[string][]string {
	if len(security) == 0 {
		return nil
	}
	result := make([]map[string][]string, 0, len(security))
	for _, item := range security {
		name, scopes, hasScopes := strings.Cut(item, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		values := []string(nil)
		if hasScopes {
			values = splitTag(scopes)
		}
		result = append(result, map[string][]string{name: values})
	}
	return result
}

func addSecuritySchemes(components map[string]any, security []string) {
	if len(security) == 0 {
		return
	}
	schemes, _ := components["securitySchemes"].(map[string]any)
	if schemes == nil {
		schemes = make(map[string]any)
		components["securitySchemes"] = schemes
	}
	for _, item := range security {
		name, _, _ := strings.Cut(item, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := schemes[name]; !exists {
			schemes[name] = map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}
		}
	}
}

func addSchemaComponents(components map[string]any, fieldType reflect.Type) {
	if fieldType == nil {
		return
	}
	fieldType = indirectType(fieldType)
	if fieldType.Kind() != reflect.Struct || fieldType.Name() == "" || fieldType == reflect.TypeOf(time.Time{}) {
		return
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		schemas = make(map[string]any)
		components["schemas"] = schemas
	}
	if _, exists := schemas[fieldType.Name()]; !exists {
		schemas[fieldType.Name()] = schemaForType(fieldType)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func fieldTypeAt(structType reflect.Type, index []int) reflect.Type {
	for _, part := range index {
		structType = indirectType(structType).Field(part).Type
	}
	return structType
}

func fieldStructAt(structType reflect.Type, index []int) reflect.StructField {
	var field reflect.StructField
	for _, part := range index {
		field = indirectType(structType).Field(part)
		structType = field.Type
	}
	return field
}

func fieldIsRequired(field reflect.StructField) bool {
	for _, tagName := range []string{"binding", "validate"} {
		for _, option := range strings.Split(field.Tag.Get(tagName), ",") {
			if strings.TrimSpace(option) == "required" {
				return true
			}
		}
	}
	return false
}

func fieldTagExists(field reflect.StructField, name string) bool {
	_, ok := field.Tag.Lookup(name)
	return ok
}

func hasAnyParameterTag(field reflect.StructField) bool {
	for _, tag := range []string{"path", "query", "header", "cookie", "form", "file"} {
		if fieldTagExists(field, tag) {
			return true
		}
	}
	return false
}

func fieldTagName(field reflect.StructField, tag, fallback string) (string, bool) {
	raw, ok := field.Tag.Lookup(tag)
	if !ok {
		return fallback, false
	}
	name := strings.TrimSpace(strings.Split(raw, ",")[0])
	if name == "-" {
		return "", true
	}
	if name == "" {
		return fallback, true
	}
	return name, true
}

func hasBindingTags(field reflect.StructField) bool {
	for _, tag := range []string{"path", "query", "header", "cookie", "form", "file", "json"} {
		if fieldTagExists(field, tag) {
			return true
		}
	}
	return false
}

func isEmbeddedStruct(fieldType reflect.Type) bool {
	return indirectType(fieldType).Kind() == reflect.Struct
}

func isMetaField(fieldType reflect.Type) bool {
	fieldType = indirectType(fieldType)
	return fieldType != nil && fieldType.Kind() == reflect.Struct && fieldType.Name() == "Meta"
}

func indirectType(fieldType reflect.Type) reflect.Type {
	for fieldType != nil && fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	return fieldType
}

func splitTag(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func integerFormat(bits int) string {
	if bits <= 32 {
		return "int32"
	}
	return "int64"
}

func floatFormat(bits int) string {
	if bits <= 32 {
		return "float"
	}
	return "double"
}
