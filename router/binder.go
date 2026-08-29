// Package router provides route contracts and compiled request binding plans.
package router

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
)

// NewBinder creates a reusable binding plan for a pointer-to-struct request.
func NewBinder(requestType reflect.Type) (*Binder, error) {
	if requestType == nil || requestType.Kind() != reflect.Ptr || requestType.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("request type must be a pointer to a struct")
	}
	binder := &Binder{MaxMultipartMemory: 32 << 20}
	if err := collectFieldBindings(requestType.Elem(), nil, &binder.fields); err != nil {
		return nil, err
	}
	binder.Fields = cloneFieldBindings(binder.fields)
	for _, field := range binder.fields {
		if field.Query != "" {
			binder.hasQuery = true
			break
		}
	}
	binder.hasBindingRules = hasValidationTag(requestType.Elem(), "binding", make(map[reflect.Type]struct{}))
	binder.hasValidateRules = hasValidationTag(requestType.Elem(), "validate", make(map[reflect.Type]struct{}))
	return binder, nil
}

// HasBindingRules reports whether binding validation tags were declared.
func (binder *Binder) HasBindingRules() bool { return binder != nil && binder.hasBindingRules }

// HasValidateRules reports whether validate tags were declared.
func (binder *Binder) HasValidateRules() bool { return binder != nil && binder.hasValidateRules }

// Bind applies the compiled plan to target. JSON uses the request Content-Type;
// scalar sources use the FieldBinding plan and default values.
func (binder *Binder) Bind(ginContext *gin.Context, target any) error {
	request := ginContext.Request
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "request target must be a non-nil pointer"}
	}
	if len(binder.fields) == 0 && (request.Body == nil || request.Body == http.NoBody) && request.Header.Get("Content-Type") == "" {
		return nil
	}
	contentType := normalizeContentType(request.Header.Get("Content-Type"))
	if err := bindJSON(request, target, contentType); err != nil {
		return err
	}
	if err := parseFormBody(request, binder.MaxMultipartMemory, contentType); err != nil {
		return err
	}
	return binder.bindSources(ginContext, targetValue.Elem())
}

func (binder *Binder) bindSources(ginContext *gin.Context, target reflect.Value) error {
	request := ginContext.Request
	for _, field := range binder.fields {
		if bound, err := bindFile(target, request, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if bound, err := bindPath(target, ginContext, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if bound, err := bindQuery(target, request, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if bound, err := bindHeader(target, request, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if bound, err := bindCookie(target, request, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if bound, err := bindForm(target, request, field); err != nil || bound {
			if err != nil {
				return err
			}
			continue
		}
		if field.HasDefault {
			if _, err := bindValues(target, field, "default", []string{field.Default}); err != nil {
				return err
			}
		}
	}
	return nil
}
