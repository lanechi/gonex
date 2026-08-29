package router

import (
	"fmt"
	"net/http"
	"reflect"
)

func bindFile(target reflect.Value, request *http.Request, field FieldBinding) (bool, error) {
	name := field.File
	value := fieldValue(target, field.Index, false)
	if name == "" && field.Form != "" && value.IsValid() && isMultipartFileType(value.Type()) {
		name = field.Form
	}
	if name == "" || request.MultipartForm == nil {
		return false, nil
	}
	files := request.MultipartForm.File[name]
	if len(files) == 0 {
		return false, nil
	}
	value = fieldValue(target, field.Index, true)
	if !value.IsValid() || !value.CanSet() {
		return false, nil
	}
	if err := assignFiles(value, files); err != nil {
		return false, &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: fmt.Sprintf("invalid file for %s", name), Cause: err}
	}
	return true, nil
}

func bindValues(target reflect.Value, field FieldBinding, name string, values []string) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	value := fieldValue(target, field.Index, false)
	if !value.IsValid() {
		value = fieldValue(target, field.Index, true)
	}
	if !value.IsValid() || !value.CanSet() {
		return false, nil
	}
	return assignBinding(value, name, values)
}
