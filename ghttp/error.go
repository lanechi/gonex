package ghttp

import (
	"fmt"
	"net/http"
)

// Error is the framework's structured application error.
type Error struct {
	Code       int
	HTTPStatus int
	Message    string
	Cause      error
	Details    any
}

func (err *Error) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause == nil {
		return err.Message
	}
	return fmt.Sprintf("%s: %v", err.Message, err.Cause)
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// NewError constructs a structured application error.
func NewError(code, status int, message string) *Error {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return &Error{Code: code, HTTPStatus: status, Message: message}
}
