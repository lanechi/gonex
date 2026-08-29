package router

import (
	"net/http"
	"reflect"
)

func bindHeader(target reflect.Value, request *http.Request, field FieldBinding) (bool, error) {
	if field.Header == "" {
		return false, nil
	}
	return bindValues(target, field, field.Header, request.Header.Values(field.Header))
}
