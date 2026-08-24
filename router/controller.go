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

// Definition is the framework-owned route representation.
type Definition struct {
	Method         string
	Path           string
	Controller     any
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
	MethodValue    reflect.Value
	Binder         *Binder
}

// ScanController validates and converts a controller into route definitions.
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
		if err := validateControllerMethod(method.Type()); err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		requestType := method.Type().In(1)
		metadata, err := ParseMeta(requestType)
		if err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		binder, err := NewBinder(requestType)
		if err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		if err := ValidatePathBindings(metadata.Path, binder.Fields); err != nil {
			return nil, fmt.Errorf("invalid controller method %s.%s: %w", controllerName, methodName, err)
		}
		var responseType reflect.Type
		if method.Type().NumOut() == 2 {
			responseType = method.Type().Out(0)
		}
		routes = append(routes, Definition{
			Method: metadata.Method, Path: metadata.Path, Controller: controller, ControllerName: controllerName, Action: methodName,
			RequestType: requestType, ResponseType: responseType, Tags: metadata.Tags, Summary: metadata.Summary,
			Description: metadata.Description, OperationID: metadata.OperationID, Deprecated: metadata.Deprecated,
			Security: metadata.Security, Produces: metadata.Produces, Consumes: metadata.Consumes,
			MethodValue: method, Binder: binder,
		})
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("controller %s has no exported methods", controllerName)
	}
	return routes, nil
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
		return fmt.Errorf("expected func(context.Context, *Request) (*Response, error) or func(context.Context, *Request) error; got %s", methodType)
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
		if responseType.Kind() != reflect.Ptr || responseType.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("first return value must be a pointer to a response struct; got %s", responseType)
		}
	default:
		return fmt.Errorf("expected one or two return values; got %d", methodType.NumOut())
	}
	return nil
}
