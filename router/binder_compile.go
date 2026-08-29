package router

import (
	"fmt"
	"reflect"
	"strings"
)

func hasValidationTag(fieldType reflect.Type, tag string, seen map[reflect.Type]struct{}) bool {
	fieldType = indirectType(fieldType)
	if fieldType == nil || fieldType.Kind() != reflect.Struct {
		return false
	}
	if _, ok := seen[fieldType]; ok {
		return false
	}
	seen[fieldType] = struct{}{}
	defer delete(seen, fieldType)
	for i := 0; i < fieldType.NumField(); i++ {
		field := fieldType.Field(i)
		if raw, ok := field.Tag.Lookup(tag); ok && strings.TrimSpace(raw) != "" {
			return true
		}
		if hasValidationTag(field.Type, tag, seen) {
			return true
		}
	}
	return false
}

func collectFieldBindings(structType reflect.Type, prefix []int, fields *[]FieldBinding) error {
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		index := append(append([]int(nil), prefix...), i)
		if field.Anonymous && isMetaField(field.Type) {
			continue
		}
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		if field.Anonymous && isEmbeddedStruct(field.Type) && !hasBindingTags(field) {
			if err := collectFieldBindings(indirectType(field.Type), index, fields); err != nil {
				return err
			}
			continue
		}
		binding := FieldBinding{Index: index}
		binding.Path, _ = fieldTagName(field, "path", "")
		binding.Query, _ = fieldTagName(field, "query", "")
		binding.Header, _ = fieldTagName(field, "header", "")
		binding.Cookie, _ = fieldTagName(field, "cookie", "")
		binding.Form, _ = fieldTagName(field, "form", "")
		binding.File, _ = fieldTagName(field, "file", "")
		if binding.File != "" && !isMultipartFileType(field.Type) {
			return fmt.Errorf("field %s uses file binding but has unsupported type %s", field.Name, field.Type)
		}
		usesString := binding.Path != "" || binding.Query != "" || binding.Header != "" || binding.Cookie != "" || binding.Form != ""
		if usesString && !isMultipartFileType(field.Type) && !supportsStringBinding(field.Type) {
			return fmt.Errorf("field %s has unsupported binding type %s", field.Name, field.Type)
		}
		if raw, ok := fieldDefaultTag(field); ok && usesString {
			if binding.Path != "" {
				return fmt.Errorf("field %s cannot use default with path binding", field.Name)
			}
			value := reflect.New(field.Type).Elem()
			if err := assignStrings(value, []string{raw}); err != nil {
				return fmt.Errorf("field %s has invalid default %q: %w", field.Name, raw, err)
			}
			binding.Default, binding.HasDefault = raw, true
		}
		if binding.Path != "" || binding.Query != "" || binding.Header != "" || binding.Cookie != "" || binding.Form != "" || binding.File != "" {
			*fields = append(*fields, binding)
		}
	}
	return nil
}
