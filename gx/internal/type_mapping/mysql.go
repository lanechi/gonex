package typemapping

import "strings"

// MySQLMapper maps MySQL-compatible integer, numeric, text, binary, JSON and
// temporal types, including modifiers such as unsigned and tinyint(1).
type MySQLMapper struct{}

func (MySQLMapper) Map(column Column) (string, bool) {
	for _, candidate := range typeCandidates(column) {
		normalized := normalizeType(candidate)
		base := baseType(normalized)
		unsigned := isUnsigned(column)
		detail := normalized
		if strings.TrimSpace(column.ColumnType) != "" {
			detail = normalizeType(column.ColumnType)
		}
		switch base {
		case "bool", "boolean":
			return "bool", true
		case "tinyint":
			if size, ok := firstSize(detail); ok && size == 1 {
				return "bool", true
			}
			if unsigned {
				return "uint8", true
			}
			return "int8", true
		case "smallint":
			if unsigned {
				return "uint16", true
			}
			return "int16", true
		case "mediumint", "int", "integer":
			if unsigned {
				return "uint32", true
			}
			return "int32", true
		case "bigint":
			if unsigned {
				return "uint64", true
			}
			return "int64", true
		case "float":
			return "float32", true
		case "double", "double precision":
			return "float64", true
		case "decimal", "numeric":
			return "decimal.Decimal", true
		case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
			return "string", true
		case "date", "datetime", "timestamp", "time":
			return "time.Time", true
		case "year":
			return "time.Time", true
		case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
			return "[]byte", true
		case "geometry", "point", "linestring", "polygon", "multipoint", "multilinestring", "multipolygon", "geometrycollection":
			return "[]byte", true
		case "json":
			return "datatypes.JSON", true
		case "bit":
			if size, ok := firstSize(normalized); ok && size == 1 {
				return "bool", true
			}
			return "[]byte", true
		}
		if strings.HasPrefix(base, "enum") || strings.HasPrefix(base, "set") {
			return "string", true
		}
	}
	return "", false
}
