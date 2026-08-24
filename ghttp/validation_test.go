package ghttp

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestWithValidatorRunsBindingAndValidateTags(t *testing.T) {
	bindingValidation := validator.New()
	bindingValidation.SetTagName("binding")
	validateValidation := validator.New()
	var bindingCalls, validateCalls int
	if err := bindingValidation.RegisterValidation("binding_rule", func(validator.FieldLevel) bool {
		bindingCalls++
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateValidation.RegisterValidation("validate_rule", func(validator.FieldLevel) bool {
		validateCalls++
		return true
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(WithBindingValidator(bindingValidation), WithValidator(validateValidation))
	request := struct {
		Binding  string `binding:"binding_rule"`
		Validate string `validate:"validate_rule"`
	}{Binding: "binding", Validate: "validate"}

	if err := server.validateRequest(&request, true, true); err != nil {
		t.Fatal(err)
	}
	if bindingCalls != 1 || validateCalls != 1 {
		t.Fatalf("binding calls=%d, validate calls=%d", bindingCalls, validateCalls)
	}
}

func TestWithValidatorPreservesNestedCrossFieldAndFieldLevelContext(t *testing.T) {
	type nestedRequest struct {
		Mode     string
		Expected string
		Value    string `binding:"required_if=Mode enabled,parent_context" validate:"eqfield=Expected,parent_context"`
	}
	type request struct {
		Marker string
		Nested nestedRequest
	}

	bindingValidation := validator.New()
	bindingValidation.SetTagName("binding")
	validateValidation := validator.New()
	var calls int
	parentContext := func(level validator.FieldLevel) bool {
		calls++
		parent := level.Parent()
		top := level.Top()
		for top.Kind() == reflect.Pointer {
			top = top.Elem()
		}
		return parent.FieldByName("Expected").String() == level.Field().String() && top.FieldByName("Marker").String() == "root"
	}
	if err := bindingValidation.RegisterValidation("parent_context", parentContext); err != nil {
		t.Fatal(err)
	}
	if err := validateValidation.RegisterValidation("parent_context", parentContext); err != nil {
		t.Fatal(err)
	}
	server := NewServer(WithBindingValidator(bindingValidation), WithValidator(validateValidation))
	value := &request{
		Marker: "root",
		Nested: nestedRequest{Mode: "enabled", Expected: "value", Value: "value"},
	}

	if err := server.validateRequest(value, true, true); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("parent context calls=%d, want 2", calls)
	}
}

func TestWithValidatorReportsBindingAndValidateFieldPaths(t *testing.T) {
	type nestedRequest struct {
		Binding  string `binding:"required"`
		Validate string `validate:"required"`
	}
	tests := []struct {
		name      string
		request   nestedRequest
		wantField string
	}{
		{name: "binding", request: nestedRequest{Validate: "valid"}, wantField: "Nested.Binding"},
		{name: "validate", request: nestedRequest{Binding: "valid"}, wantField: "Nested.Validate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := NewServer(WithValidator(validator.New()))
			request := struct {
				Nested nestedRequest
			}{Nested: test.request}
			err := server.validateRequest(&request, true, true)
			var frameworkError *Error
			if !errors.As(err, &frameworkError) {
				t.Fatalf("error=%T %v", err, err)
			}
			details, ok := frameworkError.Details.([]ValidationFieldError)
			if !ok || len(details) != 1 {
				t.Fatalf("details=%#v", frameworkError.Details)
			}
			if details[0].Field != test.wantField {
				t.Fatalf("field=%q, want %q", details[0].Field, test.wantField)
			}
		})
	}
}

func TestWithValidatorKeepsRepeatedNestedFieldPath(t *testing.T) {
	type nestedRequest struct {
		Name string `validate:"required"`
	}
	request := struct {
		First  nestedRequest
		Second nestedRequest
	}{First: nestedRequest{Name: "valid"}}
	server := NewServer(WithValidator(validator.New()))

	err := server.validateRequest(&request, false, true)
	var frameworkError *Error
	if !errors.As(err, &frameworkError) {
		t.Fatalf("error=%T %v", err, err)
	}
	details, ok := frameworkError.Details.([]ValidationFieldError)
	if !ok || len(details) != 1 || details[0].Field != "Second.Name" {
		t.Fatalf("details=%#v", frameworkError.Details)
	}
}

func TestWithValidatorRestoresCallerTagName(t *testing.T) {
	validation := validator.New()
	validation.SetTagName("check")
	server := NewServer(WithValidator(validation))
	request := struct {
		Binding string `binding:"required"`
	}{Binding: "valid"}
	if err := server.validateRequest(&request, true, false); err != nil {
		t.Fatal(err)
	}

	external := struct {
		Value string `check:"required"`
	}{}
	if err := validation.Struct(&external); err == nil {
		t.Fatal("custom validator tag name was not restored")
	}
}

func TestWithValidatorHandlesSelfReferentialRequest(t *testing.T) {
	type recursiveRequest struct {
		Name string `binding:"required" validate:"required"`
		Self *recursiveRequest
	}
	request := &recursiveRequest{Name: "valid"}
	request.Self = request
	server := NewServer(WithValidator(validator.New()))

	if err := server.validateRequest(request, true, true); err != nil {
		t.Fatal(err)
	}
}

func TestWithValidatorRunsCustomStructLevelValidation(t *testing.T) {
	type structLevelRequest struct {
		Name string
	}
	validation := validator.New()
	validation.RegisterStructValidation(func(level validator.StructLevel) {
		level.ReportError(level.Current().FieldByName("Name").Interface(), "Name", "Name", "struct_rule", "")
	}, structLevelRequest{})
	server := NewServer(WithValidator(validation))

	err := server.validateRequest(&structLevelRequest{Name: "invalid"}, false, false)
	var frameworkError *Error
	if !errors.As(err, &frameworkError) {
		t.Fatalf("error=%T %v", err, err)
	}
	details, ok := frameworkError.Details.([]ValidationFieldError)
	if !ok || len(details) != 1 || details[0].Field != "Name" || details[0].Tag != "struct_rule" {
		t.Fatalf("details=%#v", frameworkError.Details)
	}
}

func TestWithValidatorRunsStructLevelRulesOnceAcrossBothTags(t *testing.T) {
	type request struct {
		Binding  string `binding:"required"`
		Validate string `validate:"required"`
	}
	validation := validator.New()
	calls := 0
	validation.RegisterStructValidation(func(validator.StructLevel) {
		calls++
	}, request{})
	server := NewServer(WithValidator(validation))

	if err := server.validateRequest(&request{Binding: "valid", Validate: "valid"}, true, true); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("struct-level calls=%d, want 1", calls)
	}
}

func TestWithValidatorMapsInvalidTargetToInternalError(t *testing.T) {
	server := NewServer(WithValidator(validator.New()))
	err := server.validateRequest(nil, false, false)
	var frameworkError *Error
	if !errors.As(err, &frameworkError) {
		t.Fatalf("error=%T %v", err, err)
	}
	if frameworkError.Code != 50002 || frameworkError.HTTPStatus != 500 {
		t.Fatalf("error=%#v", frameworkError)
	}
}

func TestWithValidatorsRejectsSharedMutableInstance(t *testing.T) {
	validation := validator.New()
	server := NewServer(WithBindingValidator(validation), WithValidator(validation))
	if err := server.Err(); err == nil || !strings.Contains(err.Error(), "must be independent") {
		t.Fatalf("server error = %v", err)
	}
}

func TestWithValidatorSupportsConcurrentCallerUse(t *testing.T) {
	validation := validator.New()
	server := NewServer(WithValidator(validation))
	value := &struct {
		Name string `validate:"required"`
	}{Name: "valid"}

	const workers = 16
	const iterations = 100
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range iterations {
				if err := server.validateRequest(value, false, true); err != nil {
					t.Errorf("server validation: %v", err)
					return
				}
				if err := validation.Struct(value); err != nil {
					t.Errorf("caller validation: %v", err)
					return
				}
			}
		}()
	}
	group.Wait()
}
