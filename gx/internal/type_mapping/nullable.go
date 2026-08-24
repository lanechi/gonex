package typemapping

import "strings"

func applyNullable(column Column, fieldType string) string {
	if (!column.Nullable && !column.HasDefault) || strings.HasPrefix(fieldType, "*") || noPointerType(fieldType) {
		return fieldType
	}
	return "*" + fieldType
}
