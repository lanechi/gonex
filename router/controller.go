package router

import (
	"context"
	"fmt"
	"reflect"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// RouteRuntime contains objects needed only to execute a registered route.
// Documentation and route snapshots should use RouteMetadata instead.
type RouteRuntime struct {
	MethodValue reflect.Value
	Binder      *Binder
}

// Definition is the framework-owned route representation.
type Definition struct {
	Metadata RouteMetadata
	Runtime  RouteRuntime
}

// ScanController scans and converts a controller into route definitions.
// Path bindings are validated when the definitions are registered, after any
// RouterGroup prefix has been applied.
func ScanController(controller any) ([]Definition, error) {
	value := reflect.ValueOf(controller)
	if !value.IsValid() {
		return nil, fmt.Errorf("controller is nil")
	}
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return nil, fmt.Errorf("controller %T must be a non-nil pointer", controller)
	}
	typeOfController := value.Type()
	controllerName := typeOfController.Elem().Name()
	if controllerName == "" {
		controllerName = typeOfController.String()
	}
	routes := make([]Definition, 0, value.NumMethod())
	for index := 0; index < value.NumMethod(); index++ {
		method := value.Method(index)
		methodName := typeOfController.Method(index).Name
		methodType := method.Type()
		if methodType.NumIn() < 2 {
			continue
		}
		requestType := methodType.In(1)
		if requestType.Kind() != reflect.Ptr || requestType.Elem().Kind() != reflect.Struct {
			continue
		}
		if _, ok := findMetaField(requestType.Elem()); !ok {
			continue
		}
		if err := validateControllerMethod(methodType); err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		metadata, err := ParseMeta(requestType)
		if err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		binder, err := NewBinder(requestType)
		if err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		var responseType reflect.Type
		if method.Type().NumOut() == 2 {
			responseType = method.Type().Out(0)
		}
		metadata.RequestType = requestType
		metadata.ResponseType = responseType
		metadata.Bindings = cloneFieldBindings(binder.fields)
		metadata.ControllerName = controllerName
		metadata.Action = methodName
		routes = append(routes, Definition{
			Metadata: metadata,
			Runtime:  RouteRuntime{MethodValue: method, Binder: binder},
		})
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("controller %s has no exported methods", controllerName)
	}
	return routes, nil
}

func cloneFieldBindings(fields []FieldBinding) []FieldBinding {
	clone := append([]FieldBinding(nil), fields...)
	for index := range clone {
		clone[index].Index = append([]int(nil), clone[index].Index...)
	}
	return clone
}

// ValidatePathBindings verifies that Gin path wildcards and request path tags
// form a one-to-one mapping.
func ValidatePathBindings(path string, fields []FieldBinding) error {
	parameters, err := pathParameterNames(path)
	if err != nil {
		return err
	}
	bindings := make(map[string]struct{})
	for _, field := range fields {
		if field.Path == "" {
			continue
		}
		if _, exists := bindings[field.Path]; exists {
			return fmt.Errorf("path parameter %q is bound by more than one field", field.Path)
		}
		bindings[field.Path] = struct{}{}
		if _, exists := parameters[field.Path]; !exists {
			return fmt.Errorf("path binding %q is not declared by route %q", field.Path, path)
		}
	}
	for parameter := range parameters {
		if _, exists := bindings[parameter]; !exists {
			return fmt.Errorf("route path parameter %q has no matching request field", parameter)
		}
	}
	return nil
}

func validateControllerMethod(methodType reflect.Type) error {
	if methodType.NumIn() != 2 {
		return fmt.Errorf("expected func(context.Context, *Request) (Response, error) or func(context.Context, *Request) error; got %s", methodType)
	}
	if methodType.In(0) != contextType {
		return fmt.Errorf("first argument must be context.Context; got %s", methodType.In(0))
	}
	requestType := methodType.In(1)
	if requestType.Kind() != reflect.Ptr || requestType.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("second argument must be a pointer to a request struct; got %s", requestType)
	}
	switch methodType.NumOut() {
	case 1:
		if methodType.Out(0) != errorType {
			return fmt.Errorf("single return value must be error; got %s", methodType.Out(0))
		}
	case 2:
		if methodType.Out(1) != errorType {
			return fmt.Errorf("second return value must be error; got %s", methodType.Out(1))
		}
		responseType := methodType.Out(0)
		if err := validateResponseType(responseType); err != nil {
			return err
		}
	default:
		return fmt.Errorf("expected one or two return values; got %d", methodType.NumOut())
	}
	return nil
}

func validateResponseType(responseType reflect.Type) error {
	if responseType == errorType || responseType.Implements(errorType) {
		return fmt.Errorf("first return value must be a JSON-encodable response type, not error; got %s", responseType)
	}
	responseKind := responseType
	for responseKind.Kind() == reflect.Ptr {
		responseKind = responseKind.Elem()
	}
	switch responseKind.Kind() {
	case reflect.Array, reflect.Bool, reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Map, reflect.Interface, reflect.Slice, reflect.String,
		reflect.Struct, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Uintptr:
		return nil
	default:
		return fmt.Errorf("first return value must be a JSON-encodable response type (value or pointer); got %s", responseType)
	}
}
