package typemapping

// SQLServerMapper maps SQL Server native types to idiomatic Go types.
type SQLServerMapper struct{}

func (SQLServerMapper) Map(column Column) (string, bool) {
	for _, candidate := range typeCandidates(column) {
		switch baseType(candidate) {
		case "tinyint":
			return "uint8", true
		case "smallint":
			return "int16", true
		case "int", "integer":
			return "int32", true
		case "bigint":
			return "int64", true
		case "bit":
			return "bool", true
		case "real":
			return "float32", true
		case "float":
			return "float64", true
		case "decimal", "numeric", "money", "smallmoney":
			return "decimal.Decimal", true
		case "char", "varchar", "text", "nchar", "nvarchar", "ntext":
			return "string", true
		case "binary", "varbinary", "image", "timestamp", "rowversion", "geography", "geometry":
			return "[]byte", true
		case "date", "time", "datetime", "datetime2", "smalldatetime", "datetimeoffset":
			return "time.Time", true
		case "uniqueidentifier":
			return "uuid.UUID", true
		case "xml":
			return "string", true
		case "hierarchyid", "sql_variant":
			return "string", true
		}
	}
	return "", false
}
