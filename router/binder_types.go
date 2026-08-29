package router

import (
	"fmt"
)

// BindingError describes a request decoding failure without coupling this
// package to the root server error type.
type BindingError struct {
	Code       int
	HTTPStatus int
	Message    string
	Cause      error
}

func (err *BindingError) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause == nil {
		return err.Message
	}
	return fmt.Sprintf("%s: %v", err.Message, err.Cause)
}

func (err *BindingError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// FieldBinding is immutable request binding metadata used by route inspection
// and documentation. Runtime Binder state remains private to the router package.
type FieldBinding struct {
	Index      []int
	Path       string
	Query      string
	Header     string
	Cookie     string
	Form       string
	Default    string
	HasDefault bool
	File       string
}

// Binder executes the request binding plan compiled during route registration.
// Fields is a compatibility snapshot for inspection; mutating it does not alter
// the private runtime binding plan.
type Binder struct {
	Fields             []FieldBinding
	MaxMultipartMemory int64
	fields             []FieldBinding
	hasQuery           bool
	hasBindingRules    bool
	hasValidateRules   bool
}
