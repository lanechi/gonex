package openapi

import (
	"encoding/json"
	"mime/multipart"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/lanechi/gonex/router"
)

var multipartFileHeaderType = reflect.TypeOf((*multipart.FileHeader)(nil))

func schemaForType(t reflect.Type) map[string]any {
	return schemaForTypeWithSeen(t, map[reflect.Type]bool{})
}
func schemaForTypeWithSeen(t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	if t == multipartFileHeaderType {
		return map[string]any{"type": "string", "format": "binary"}
	}
	if t.Kind() == reflect.Slice && t.Elem() == multipartFileHeaderType {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "binary"}}
	}
	t = indirectType(t)
	if seen[t] {
		return map[string]any{"type": "object"}
	}
	seen[t] = true
	defer delete(seen, t)
	if t == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	switch t.Kind() {
	case reflect.Struct:
		p := map[string]any{}
		req := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && isMetaField(f.Type) || f.PkgPath != "" && !f.Anonymous {
				continue
			}
			if f.Anonymous && isEmbeddedStruct(f.Type) && !fieldTagExists(f, "json") {
				req = mergeSchemaProperties(p, req, schemaForTypeWithSeen(f.Type, seen))
				continue
			}
			n, _ := fieldTagName(f, "json", f.Name)
			if n == "" {
				continue
			}
			p[n] = schemaForFieldWithSeen(f, f.Type, seen)
			if fieldIsRequired(f) {
				req = append(req, n)
			}
		}
		s := map[string]any{"type": "object", "properties": p}
		if len(req) > 0 {
			s["required"] = req
		}
		return s
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": schemaForTypeWithSeen(t.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaForTypeWithSeen(t.Elem(), seen)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer", "format": integerFormat(t.Bits())}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer", "format": integerFormat(t.Bits()), "minimum": 0}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number", "format": floatFormat(t.Bits())}
	}
	return map[string]any{}
}
func schemaForField(f reflect.StructField, t reflect.Type) map[string]any {
	return schemaForFieldWithSeen(f, t, map[reflect.Type]bool{})
}
func schemaForFieldWithSeen(f reflect.StructField, t reflect.Type, seen map[reflect.Type]bool) map[string]any {
	s := schemaForTypeWithSeen(t, seen)
	if v := firstNonEmptyTag(f, "description", "dc"); v != "" {
		s["description"] = v
	}
	if v := strings.TrimSpace(f.Tag.Get("example")); v != "" {
		s["example"] = parseJSONTagValue(v)
	}
	if v, ok := defaultTagValue(f); ok {
		s["default"] = parseJSONTagValue(strings.TrimSpace(v))
	}
	if v := strings.TrimSpace(f.Tag.Get("enum")); v != "" {
		a := []any{}
		for _, x := range splitTag(v) {
			a = append(a, parseTypedTagValue(x, t))
		}
		s["enum"] = a
	}
	for _, tag := range []string{"binding", "validate"} {
		for _, rule := range strings.Split(f.Tag.Get(tag), ",") {
			n, v, ok := strings.Cut(strings.TrimSpace(rule), "=")
			if !ok || v == "" {
				continue
			}
			switch n {
			case "gte", "min":
				s[constraintKey(t, "minimum")] = parseJSONTagValue(v)
			case "gt":
				applyExclusiveConstraint(s, t, v, true)
			case "lte", "max":
				s[constraintKey(t, "maximum")] = parseJSONTagValue(v)
			case "lt":
				applyExclusiveConstraint(s, t, v, false)
			}
		}
	}
	return s
}
func applyExclusiveConstraint(s map[string]any, t reflect.Type, raw string, min bool) {
	t = indirectType(t)
	switch t.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			return
		}
		if min {
			s[constraintKey(t, "minimum")] = v + 1
		} else if v > 0 {
			s[constraintKey(t, "maximum")] = v - 1
		}
	default:
		v := parseJSONTagValue(raw)
		if min {
			s["minimum"] = v
			s["exclusiveMinimum"] = true
		} else {
			s["maximum"] = v
			s["exclusiveMaximum"] = true
		}
	}
}
func constraintKey(t reflect.Type, key string) string {
	t = indirectType(t)
	min := strings.Contains(strings.ToLower(key), "minimum")
	switch t.Kind() {
	case reflect.String:
		if min {
			return "minLength"
		}
		return "maxLength"
	case reflect.Slice, reflect.Array:
		if min {
			return "minItems"
		}
		return "maxItems"
	case reflect.Map:
		if min {
			return "minProperties"
		}
		return "maxProperties"
	}
	return key
}
func firstNonEmptyTag(f reflect.StructField, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(f.Tag.Get(n)); v != "" {
			return v
		}
	}
	return ""
}
func parseJSONTagValue(raw string) any {
	var v any
	if json.Unmarshal([]byte(raw), &v) == nil {
		return v
	}
	if x, e := strconv.ParseBool(raw); e == nil {
		return x
	}
	if x, e := strconv.ParseInt(raw, 10, 64); e == nil {
		return x
	}
	if x, e := strconv.ParseFloat(raw, 64); e == nil {
		return x
	}
	return raw
}
func parseTypedTagValue(raw string, t reflect.Type) any {
	t = indirectType(t)
	switch t.Kind() {
	case reflect.Bool:
		if v, e := strconv.ParseBool(raw); e == nil {
			return v
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, e := strconv.ParseInt(raw, 10, 64); e == nil {
			return v
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if v, e := strconv.ParseUint(raw, 10, 64); e == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, e := strconv.ParseFloat(raw, 64); e == nil {
			return v
		}
	}
	return raw
}
func addSchemaComponents(c map[string]any, t reflect.Type) {
	if t == nil {
		return
	}
	t = indirectType(t)
	if t.Kind() != reflect.Struct || t.Name() == "" || t == reflect.TypeOf(time.Time{}) {
		return
	}
	schemas, _ := c["schemas"].(map[string]any)
	if schemas == nil {
		schemas = map[string]any{}
		c["schemas"] = schemas
	}
	if _, ok := schemas[t.Name()]; !ok {
		schemas[t.Name()] = schemaForType(t)
	}
}
func fieldIsRequired(f reflect.StructField) bool {
	for _, tag := range []string{"binding", "validate"} {
		for _, o := range strings.Split(f.Tag.Get(tag), ",") {
			if strings.TrimSpace(o) == "required" {
				return true
			}
		}
	}
	return false
}
func fieldTagExists(f reflect.StructField, n string) bool { _, ok := f.Tag.Lookup(n); return ok }
func fieldTagName(f reflect.StructField, tag, fallback string) (string, bool) {
	raw, ok := f.Tag.Lookup(tag)
	if !ok {
		return fallback, false
	}
	n := strings.TrimSpace(strings.Split(raw, ",")[0])
	if n == "-" {
		return "", true
	}
	if n == "" {
		return fallback, true
	}
	return n, true
}
func defaultTagValue(f reflect.StructField) (string, bool) {
	if v, ok := f.Tag.Lookup("default"); ok {
		return v, true
	}
	if v, ok := f.Tag.Lookup("d"); ok {
		return v, true
	}
	return "", false
}
func hasBindingTags(f reflect.StructField) bool {
	for _, n := range []string{"path", "query", "header", "cookie", "form", "file", "json"} {
		if fieldTagExists(f, n) {
			return true
		}
	}
	return false
}
func isEmbeddedStruct(t reflect.Type) bool { return indirectType(t).Kind() == reflect.Struct }
func isMetaField(t reflect.Type) bool {
	t = indirectType(t)
	return t == reflect.TypeOf(router.Meta{})
}
func indirectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
func splitTag(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}
func integerFormat(bits int) string {
	if bits <= 32 {
		return "int32"
	}
	return "int64"
}
func floatFormat(bits int) string {
	if bits <= 32 {
		return "float"
	}
	return "double"
}
