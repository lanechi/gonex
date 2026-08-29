package router

import (
	"encoding"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

func assignBinding(destination reflect.Value, name string, values []string) (bool, error) {
	if name == "" || len(values) == 0 {
		return false, nil
	}
	if err := assignStrings(destination, values); err != nil {
		return true, &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: fmt.Sprintf("invalid value for %s", name), Cause: err}
	}
	return true, nil
}

func jsonBindingError(err error) *BindingError {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return &BindingError{Code: 41300, HTTPStatus: http.StatusRequestEntityTooLarge, Message: "request body is too large", Cause: err}
	}
	return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "invalid JSON request body", Cause: err}
}

func fieldValue(value reflect.Value, index []int, allocate bool) reflect.Value {
	for _, part := range index {
		for value.Kind() == reflect.Ptr {
			if value.IsNil() && allocate && value.CanSet() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			if value.IsNil() {
				return reflect.Value{}
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		value = value.Field(part)
	}
	return value
}

func assignStrings(destination reflect.Value, values []string) error {
	if destination.Kind() == reflect.Ptr {
		if destination.IsNil() {
			destination.Set(reflect.New(destination.Type().Elem()))
		}
		return assignStrings(destination.Elem(), values)
	}
	if destination.Kind() == reflect.Slice {
		if destination.Type().Elem().Kind() == reflect.Uint8 {
			destination.SetBytes([]byte(values[0]))
			return nil
		}
		result := reflect.MakeSlice(destination.Type(), len(values), len(values))
		for index := range values {
			if err := assignStrings(result.Index(index), []string{values[index]}); err != nil {
				return err
			}
		}
		destination.Set(result)
		return nil
	}
	if destination.Kind() == reflect.Array {
		if len(values) != destination.Len() {
			return fmt.Errorf("expected %d values, received %d", destination.Len(), len(values))
		}
		for index := range values {
			if err := assignStrings(destination.Index(index), []string{values[index]}); err != nil {
				return err
			}
		}
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	value := values[0]
	if destination.CanAddr() && destination.Addr().Type().Implements(textUnmarshalerType) {
		return destination.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value))
	}
	if destination.Type() == durationType {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		destination.SetInt(int64(parsed))
		return nil
	}
	switch destination.Kind() {
	case reflect.String:
		destination.SetString(value)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		destination.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(value, 10, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsed, err := strconv.ParseUint(value, 10, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported binding type %s", destination.Type())
	}
	return nil
}

var (
	multipartFileHeaderType = reflect.TypeOf((*multipart.FileHeader)(nil))
	textUnmarshalerType     = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	durationType            = reflect.TypeOf(time.Duration(0))
)

func supportsStringBinding(valueType reflect.Type) bool {
	if valueType.Kind() == reflect.Ptr {
		return supportsStringBinding(valueType.Elem())
	}
	if reflect.PointerTo(valueType).Implements(textUnmarshalerType) || valueType == durationType {
		return true
	}
	switch valueType.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice, reflect.Array:
		return valueType.Elem().Kind() == reflect.Uint8 || supportsStringBinding(valueType.Elem())
	default:
		return false
	}
}

func isMultipartFileType(valueType reflect.Type) bool {
	return valueType == multipartFileHeaderType || (valueType.Kind() == reflect.Slice && valueType.Elem() == multipartFileHeaderType)
}

func assignFiles(destination reflect.Value, files []*multipart.FileHeader) error {
	if destination.Type() == multipartFileHeaderType {
		destination.Set(reflect.ValueOf(files[0]))
		return nil
	}
	if destination.Kind() == reflect.Slice && destination.Type().Elem() == multipartFileHeaderType {
		result := reflect.MakeSlice(destination.Type(), len(files), len(files))
		for index, file := range files {
			result.Index(index).Set(reflect.ValueOf(file))
		}
		destination.Set(result)
		return nil
	}
	return fmt.Errorf("unsupported multipart file type %s", destination.Type())
}
