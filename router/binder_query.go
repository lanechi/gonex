package router

import (
	"net/http"
	"reflect"
)

func bindQuery(target reflect.Value, request *http.Request, field FieldBinding) (bool, error) {
	if field.Query == "" {
		return false, nil
	}
	return bindValues(target, field, field.Query, request.URL.Query()[field.Query])
}
