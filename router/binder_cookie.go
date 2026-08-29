package router

import (
	"net/http"
	"reflect"
)

func bindCookie(target reflect.Value, request *http.Request, field FieldBinding) (bool, error) {
	if field.Cookie == "" {
		return false, nil
	}
	cookie, err := request.Cookie(field.Cookie)
	if err != nil {
		return false, nil
	}
	return bindValues(target, field, field.Cookie, []string{cookie.Value})
}
