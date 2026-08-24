package typemapping

import "strings"

// PostgresMapper maps PostgreSQL native, alias, and array types.
type PostgresMapper struct{}

func (PostgresMapper) Map(column Column) (string, bool) {
	for _, candidate := range typeCandidates(column) {
		if element, ok := arrayElementType(candidate); ok {
			if mapped, known := postgresScalarType(element); known {
				return "[]" + mapped, true
			}
			return "[]string", true
		}
		if strings.HasPrefix(baseType(candidate), "array") {
			if element, ok := arrayElementType(column.ColumnType); ok {
				if mapped, known := postgresScalarType(element); known {
					return "[]" + mapped, true
				}
				return "[]string", true
			}
		}
		if mapped, ok := postgresScalarType(candidate); ok {
			return mapped, true
		}
	}
	return "", false
}

func postgresScalarType(value string) (string, bool) {
	switch baseType(value) {
	case "int2", "smallint", "smallserial", "serial2":
		return "int16", true
	case "int4", "integer", "serial", "serial4":
		return "int32", true
	case "int8", "bigint", "bigserial", "serial8":
		return "int64", true
	case "real", "float4":
		return "float32", true
	case "double", "float8":
		return "float64", true
	case "numeric", "decimal", "money":
		return "decimal.Decimal", true
	case "bool", "boolean":
		return "bool", true
	case "char", "bpchar", "character", "varchar", "character varying", "text", "citext":
		return "string", true
	case "uuid":
		return "uuid.UUID", true
	case "json", "jsonb":
		return "datatypes.JSON", true
	case "bytea":
		return "[]byte", true
	case "date", "time", "timetz", "timestamp", "timestamptz", "timestampz":
		return "time.Time", true
	case "interval":
		return "time.Duration", true
	case "tsvector", "tsquery", "xml", "jsonpath", "pg_lsn", "hstore", "ltree":
		return "string", true
	case "oid", "xid", "cid", "regclass", "regcollation", "regconfig", "regdictionary", "regnamespace", "regoper", "regoperator", "regproc", "regprocedure", "regrole", "regtype":
		return "uint32", true
	case "xid8":
		return "uint64", true
	case "inet", "cidr", "macaddr", "macaddr8":
		return "string", true
	case "bit", "varbit":
		return "[]byte", true
	default:
		return "", false
	}
}
