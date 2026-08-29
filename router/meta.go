// Package router contains route metadata and registration primitives.
package router

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// RouteMetadata is the normalized metadata extracted from a request type.
type RouteMetadata struct {
	Path           string
	Method         string
	ControllerName string
	Action         string
	RequestType    reflect.Type
	ResponseType   reflect.Type
	Tags           []string
	Summary        string
	Description    string
	OperationID    string
	Deprecated     bool
	Security       []string
	Produces       []string
	Consumes       []string
	// Bindings is the immutable binding contract used by documentation and
	// route inspection. It deliberately contains no executable Binder state.
	Bindings []FieldBinding
}

// Meta marks a request structure as a declarative route definition.
// It is defined here so discovery can use the package-qualified type identity.
type Meta struct{}

// ParseMeta extracts route metadata from a pointer-to-struct request type.
func ParseMeta(requestType reflect.Type) (RouteMetadata, error) {
	if requestType == nil {
		return RouteMetadata{}, fmt.Errorf("request type is nil")
	}
	if requestType.Kind() != reflect.Ptr || requestType.Elem().Kind() != reflect.Struct {
		return RouteMetadata{}, fmt.Errorf("request type %s must be a pointer to a struct", requestType)
	}
	requestStruct := requestType.Elem()
	metaField, ok := findMetaField(requestStruct)
	if !ok {
		return RouteMetadata{}, fmt.Errorf("request type %s must embed g.Meta", requestType)
	}
	path := strings.TrimSpace(metaField.Tag.Get("path"))
	if path == "" {
		return RouteMetadata{}, fmt.Errorf("request type %s Meta tag path is required", requestType)
	}
	if !strings.HasPrefix(path, "/") {
		return RouteMetadata{}, fmt.Errorf("request type %s Meta path %q must start with /", requestType, path)
	}
	if _, err := pathParameterNames(path); err != nil {
		return RouteMetadata{}, fmt.Errorf("request type %s Meta path %q is invalid: %w", requestType, path, err)
	}
	method := strings.ToUpper(strings.TrimSpace(metaField.Tag.Get("method")))
	if method == "" {
		return RouteMetadata{}, fmt.Errorf("request type %s Meta tag method is required", requestType)
	}
	if strings.ContainsAny(method, " \t\r\n,") {
		return RouteMetadata{}, fmt.Errorf("request type %s Meta method %q must contain exactly one HTTP method", requestType, method)
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE":
	default:
		return RouteMetadata{}, fmt.Errorf("request type %s Meta method %q is not a supported HTTP method", requestType, method)
	}
	deprecated := false
	if raw := strings.TrimSpace(metaField.Tag.Get("deprecated")); raw != "" {
		var err error
		deprecated, err = strconv.ParseBool(raw)
		if err != nil {
			return RouteMetadata{}, fmt.Errorf("request type %s Meta tag deprecated %q is invalid: %w", requestType, raw, err)
		}
	}
	return RouteMetadata{
		Path: path, Method: method, Tags: splitTag(metaField.Tag.Get("tags")),
		Summary: strings.TrimSpace(metaField.Tag.Get("summary")), Description: strings.TrimSpace(metaField.Tag.Get("description")),
		OperationID: strings.TrimSpace(metaField.Tag.Get("operationId")), Deprecated: deprecated,
		Security: splitTag(metaField.Tag.Get("security")), Produces: splitTag(metaField.Tag.Get("produces")), Consumes: splitTag(metaField.Tag.Get("consumes")),
	}, nil
}

func pathParameterNames(path string) (map[string]struct{}, error) {
	parameters := make(map[string]struct{})
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for index, segment := range segments {
		wildcardIndex := strings.IndexAny(segment, ":*")
		if wildcardIndex < 0 {
			continue
		}
		if wildcardIndex != 0 {
			return nil, fmt.Errorf("wildcard must occupy an entire path segment")
		}
		if len(segment) == 1 || strings.ContainsAny(segment[1:], ":*") {
			return nil, fmt.Errorf("wildcard name is required and cannot contain ':' or '*'")
		}
		if segment[0] == '*' && index != len(segments)-1 {
			return nil, fmt.Errorf("catch-all wildcard must be the final path segment")
		}
		name := segment[1:]
		if _, exists := parameters[name]; exists {
			return nil, fmt.Errorf("path parameter %q is declared more than once", name)
		}
		parameters[name] = struct{}{}
	}
	return parameters, nil
}

func findMetaField(requestStruct reflect.Type) (reflect.StructField, bool) {
	for index := 0; index < requestStruct.NumField(); index++ {
		field := requestStruct.Field(index)
		if field.Anonymous && isMetaField(field.Type) {
			return field, true
		}
	}
	return reflect.StructField{}, false
}

func isMetaField(fieldType reflect.Type) bool {
	if fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	return fieldType == reflect.TypeOf(Meta{})
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
