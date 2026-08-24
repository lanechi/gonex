package ghttp

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationFieldError struct {
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Param string `json:"param,omitempty"`
}

func newValidator(tag string) *validator.Validate {
	validation := validator.New()
	validation.SetTagName(tag)
	return validation
}

func (server *Server) validateRequest(request any, hasBindingRules, hasValidateRules bool) error {
	if !server.customBindingValidator && !server.customValidateValidator && !hasBindingRules && !hasValidateRules {
		return nil
	}
	validators := []*validator.Validate{nil, nil}
	if hasBindingRules || server.customBindingValidator {
		validators[0] = server.bindingValidator
	}
	if hasValidateRules || server.customValidateValidator {
		validators[1] = server.validateValidator
	}
	cyclePaths := validationCyclePaths(reflect.ValueOf(request))
	for _, validation := range validators {
		if validation == nil {
			continue
		}
		if err := validateRequestWith(validation, request, cyclePaths); err != nil {
			return err
		}
	}
	return nil
}

func validateRequestWith(validation *validator.Validate, request any, cyclePaths map[string]struct{}) error {
	var err error
	if len(cyclePaths) == 0 {
		err = validation.Struct(request)
	} else {
		err = validation.StructFiltered(request, func(namespace []byte) bool {
			name := string(namespace)
			for path := range cyclePaths {
				if name == path || strings.HasSuffix(name, "."+path) {
					return true
				}
			}
			return false
		})
	}
	if err == nil {
		return nil
	}
	if invalid, ok := err.(*validator.InvalidValidationError); ok {
		return &Error{Code: 50002, HTTPStatus: 500, Message: "invalid validation target", Cause: invalid}
	}
	return validationErrorForRequest(err, reflect.TypeOf(request))
}

func validationErrorForRequest(err error, requestType reflect.Type) error {
	validationErr := validationErrorAtPath(err, "")
	frameworkErr, ok := validationErr.(*Error)
	if !ok {
		return validationErr
	}
	details, ok := frameworkErr.Details.([]ValidationFieldError)
	if !ok {
		return frameworkErr
	}
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return frameworkErr
	}
	for index, fieldError := range validationErrors {
		if path := validationPathForRequest(requestType, fieldError); path != "" {
			details[index].Field = path
		}
	}
	return frameworkErr
}

func validationPathForRequest(requestType reflect.Type, fieldError validator.FieldError) string {
	namespace := strings.TrimPrefix(fieldError.StructNamespace(), ".")
	for requestType != nil && requestType.Kind() == reflect.Pointer {
		requestType = requestType.Elem()
	}
	if requestType != nil && requestType.Name() != "" {
		namespace = strings.TrimPrefix(namespace, requestType.Name()+".")
	}
	if namespace != "" {
		return namespace
	}
	if field := fieldError.StructField(); field != "" {
		return field
	}
	return fieldError.Field()
}

func validationCyclePaths(value reflect.Value) map[string]struct{} {
	paths := make(map[string]struct{})
	collectValidationCyclePaths(value, "", make(map[validationVisit]struct{}), paths)
	return paths
}

func collectValidationCyclePaths(value reflect.Value, path string, visiting map[validationVisit]struct{}, paths map[string]struct{}) {
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		visit := validationVisit{typeOf: value.Type(), pointer: value.Pointer()}
		if _, ok := visiting[visit]; ok {
			paths[path] = struct{}{}
			return
		}
		visiting[visit] = struct{}{}
		defer delete(visiting, visit)
		collectValidationCyclePaths(value.Elem(), path, visiting, paths)
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.IsExported() {
				collectValidationCyclePaths(value.Field(index), joinValidationPath(path, field.Name), visiting, paths)
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			collectValidationCyclePaths(value.Index(index), fmt.Sprintf("%s[%d]", path, index), visiting, paths)
		}
	}
}

type validationVisit struct {
	typeOf  reflect.Type
	pointer uintptr
}

func validationErrorAtPath(err error, path string) error {
	if err == nil {
		return nil
	}
	if invalid, ok := err.(*validator.InvalidValidationError); ok {
		return &Error{Code: 50002, HTTPStatus: 500, Message: "invalid validation target", Cause: invalid}
	}
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return validationError(err)
	}
	details := make([]ValidationFieldError, 0, len(validationErrors))
	for _, fieldError := range validationErrors {
		details = append(details, ValidationFieldError{
			Field: validationFieldPath(path, fieldError),
			Tag:   fieldError.Tag(),
			Param: fieldError.Param(),
		})
	}
	return &Error{
		Code:       40002,
		HTTPStatus: 400,
		Message:    "request validation failed",
		Cause:      err,
		Details:    details,
	}
}

func validationFieldPath(path string, fieldError validator.FieldError) string {
	namespace := fieldError.Namespace()
	if namespace == "" {
		namespace = fieldError.StructNamespace()
	}
	if separator := strings.IndexByte(namespace, '.'); separator >= 0 {
		namespace = namespace[separator+1:]
	}
	if namespace == "" {
		namespace = fieldError.StructField()
	}
	if namespace == "" {
		namespace = fieldError.Field()
	}
	return joinValidationPath(path, namespace)
}

func joinValidationPath(path, field string) string {
	if path == "" {
		return field
	}
	if field == "" || field == path || strings.HasSuffix(path, "."+field) {
		return path
	}
	if strings.HasPrefix(field, "[") {
		return path + field
	}
	return path + "." + field
}

func validationError(err error) *Error {
	fieldErrors := make([]ValidationFieldError, 0)
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		fieldErrors = make([]ValidationFieldError, 0, len(validationErrors))
		for _, fieldError := range validationErrors {
			fieldErrors = append(fieldErrors, ValidationFieldError{
				Field: fieldError.Field(),
				Tag:   fieldError.Tag(),
				Param: fieldError.Param(),
			})
		}
	}
	return &Error{
		Code:       40002,
		HTTPStatus: 400,
		Message:    "request validation failed",
		Cause:      err,
		Details:    fieldErrors,
	}
}

func (fieldError ValidationFieldError) String() string {
	if fieldError.Param == "" {
		return fmt.Sprintf("%s (%s)", fieldError.Field, fieldError.Tag)
	}
	return fmt.Sprintf("%s (%s=%s)", fieldError.Field, fieldError.Tag, fieldError.Param)
}
