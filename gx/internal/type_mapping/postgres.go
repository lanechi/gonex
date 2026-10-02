package typemapping

// PostgresMapper maps PostgreSQL built-in, alias, and array types to existing
// Go, GORM, and pgx types that can be used directly by GORM. PostgreSQL arrays
// use pgtype.Array[T] rather than a slice: GORM binds structs as one driver
// argument while ordinary slices are expanded as SQL value lists before pgx
// can apply its ArrayCodec.
type PostgresMapper struct{}

func (PostgresMapper) Map(column Column) (string, bool) {
	if column.Postgres != nil {
		if column.Postgres.IsArray {
			mapped, known := postgresScalarType(column.Postgres.ElementName)
			if !known {
				return "", false
			}
			return "pgtype.Array[" + mapped + "]", true
		}
		if mapped, ok := postgresScalarType(column.Postgres.Name); ok {
			return mapped, true
		}
	}

	for _, candidate := range typeCandidates(column) {
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
	case "numeric", "decimal":
		return "decimal.Decimal", true
	case "money":
		// PostgreSQL money has locale-sensitive text formatting. Keep the exact
		// database representation instead of pretending it is a plain decimal.
		return "string", true
	case "bool", "boolean":
		return "bool", true
	case "char", "bpchar", "character", "varchar", "character varying", "text", "citext", "name":
		return "string", true
	case "uuid":
		return "datatypes.UUID", true
	case "json", "jsonb":
		return "datatypes.JSON", true
	case "bytea":
		return "[]byte", true
	case "date":
		return "datatypes.Date", true
	case "time":
		// pgtype.Time preserves PostgreSQL's valid 24:00:00 value.
		return "pgtype.Time", true
	case "timetz":
		// pgx intentionally does not provide a timetz value type because
		// PostgreSQL itself discourages time with time zone.
		return "string", true
	case "timestamp", "timestamptz", "timestampz":
		return "time.Time", true
	case "interval":
		return "pgtype.Interval", true
	case "point":
		return "pgtype.Point", true
	case "line":
		return "pgtype.Line", true
	case "lseg":
		return "pgtype.Lseg", true
	case "box":
		return "pgtype.Box", true
	case "path":
		return "pgtype.Path", true
	case "polygon":
		return "pgtype.Polygon", true
	case "circle":
		return "pgtype.Circle", true
	case "bit", "varbit":
		return "pgtype.Bits", true
	case "inet":
		return "netip.Addr", true
	case "cidr":
		return "netip.Prefix", true
	case "macaddr", "macaddr8":
		return "net.HardwareAddr", true
	case "hstore":
		return "pgtype.Hstore", true
	case "tsvector":
		return "pgtype.TSVector", true
	case "tsquery", "xml", "jsonpath", "pg_lsn", "ltree":
		return "string", true
	case "tid":
		return "pgtype.TID", true
	case "oid", "xid", "cid", "regclass", "regcollation", "regconfig", "regdictionary", "regnamespace", "regoper", "regoperator", "regproc", "regprocedure", "regrole", "regtype":
		return "uint32", true
	case "xid8":
		return "uint64", true
	case "int4range":
		return "pgtype.Range[int32]", true
	case "int8range":
		return "pgtype.Range[int64]", true
	case "numrange":
		return "pgtype.Range[pgtype.Numeric]", true
	case "daterange", "tsrange", "tstzrange":
		return "pgtype.Range[time.Time]", true
	case "int4multirange":
		return "pgtype.Multirange[pgtype.Range[int32]]", true
	case "int8multirange":
		return "pgtype.Multirange[pgtype.Range[int64]]", true
	case "nummultirange":
		return "pgtype.Multirange[pgtype.Range[pgtype.Numeric]]", true
	case "datemultirange", "tsmultirange", "tstzmultirange":
		return "pgtype.Multirange[pgtype.Range[time.Time]]", true
	default:
		return "", false
	}
}
