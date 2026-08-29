package router

import (
	"reflect"
	"strings"
)

func fieldTagName(field reflect.StructField, tag, fallback string) (string, bool) {
	raw, ok := field.Tag.Lookup(tag)
	if !ok {
		return fallback, false
	}
	name := strings.TrimSpace(strings.Split(raw, ",")[0])
	if name == "-" {
		return "", true
	}
	if name == "" {
		return fallback, true
	}
	return name, true
}
func fieldDefaultTag(field reflect.StructField) (string, bool) {
	if raw, ok := field.Tag.Lookup("default"); ok {
		return strings.TrimSpace(raw), true
	}
	if raw, ok := field.Tag.Lookup("d"); ok {
		return strings.TrimSpace(raw), true
	}
	return "", false
}
func hasBindingTags(field reflect.StructField) bool {
	for _, tag := range []string{"path", "query", "header", "cookie", "form", "file", "json"} {
		if _, ok := field.Tag.Lookup(tag); ok {
			return true
		}
	}
	return false
}
func isEmbeddedStruct(t reflect.Type) bool { return indirectType(t).Kind() == reflect.Struct }
func indirectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
