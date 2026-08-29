package router

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
)

func bindForm(target reflect.Value, request *http.Request, field FieldBinding) (bool, error) {
	if field.Form == "" {
		return false, nil
	}
	if len(request.PostForm) == 0 {
		_ = request.ParseForm()
	}
	values := request.PostForm[field.Form]
	if len(values) == 0 {
		values = request.Form[field.Form]
	}
	return bindValues(target, field, field.Form, values)
}

func isJSONContentType(contentType string) bool {
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}

func isFormContentType(contentType string) bool {
	return contentType == "application/x-www-form-urlencoded"
}

func isMultipartContentType(contentType string) bool { return contentType == "multipart/form-data" }

func normalizeContentType(contentType string) string {
	if separator := strings.IndexByte(contentType, ';'); separator >= 0 {
		contentType = contentType[:separator]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func parseFormBody(request *http.Request, maxMultipartMemory int64, contentType string) error {
	var err error
	switch {
	case isMultipartContentType(contentType):
		err = request.ParseMultipartForm(maxMultipartMemory)
	case isFormContentType(contentType):
		err = request.ParseForm()
	default:
		return nil
	}
	if err == nil {
		return nil
	}
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return &BindingError{Code: 41300, HTTPStatus: http.StatusRequestEntityTooLarge, Message: "form request body is too large", Cause: err}
	}
	return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "invalid form request body", Cause: err}
}
