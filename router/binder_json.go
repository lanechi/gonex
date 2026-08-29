package router

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func bindJSON(request *http.Request, target any, contentType string) error {
	if !isJSONContentType(contentType) || request.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(target); err != nil && err != io.EOF {
		return jsonBindingError(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("request body must contain exactly one JSON value")
		}
		return jsonBindingError(err)
	}
	return nil
}
