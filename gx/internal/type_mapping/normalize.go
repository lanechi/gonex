package typemapping

import (
	"strconv"
	"strings"
)

func normalizeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, "`\"")
	return strings.Join(strings.Fields(value), " ")
}

func baseType(value string) string {
	value = normalizeType(value)
	if index := strings.IndexByte(value, '('); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if fields := strings.Fields(value); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func typeCandidates(column Column) []string {
	result := make([]string, 0, 2)
	for _, value := range []string{column.DataType, column.ColumnType} {
		value = normalizeType(value)
		if value == "" {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, value)
		}
	}
	return result
}

func firstSize(value string) (int64, bool) {
	value = normalizeType(value)
	start := strings.IndexByte(value, '(')
	if start < 0 {
		return 0, false
	}
	end := strings.IndexByte(value[start+1:], ')')
	if end < 0 {
		return 0, false
	}
	end += start + 1
	parts := strings.Split(value[start+1:end], ",")
	if len(parts) == 0 {
		return 0, false
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func isUnsigned(column Column) bool {
	return column.Unsigned || strings.Contains(normalizeType(column.ColumnType), " unsigned") || strings.Contains(normalizeType(column.DataType), " unsigned")
}

func arrayElementType(value string) (string, bool) {
	value = normalizeType(value)
	if strings.HasPrefix(value, "_") && len(value) > 1 {
		return strings.TrimPrefix(value, "_"), true
	}
	if strings.HasSuffix(value, "[]") {
		return strings.TrimSpace(strings.TrimSuffix(value, "[]")), true
	}
	return "", false
}

func noPointerType(value string) bool {
	return strings.HasPrefix(value, "[]") || value == "datatypes.JSON"
}
