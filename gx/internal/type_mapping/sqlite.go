package typemapping

import "strings"

// SQLiteMapper follows SQLite type affinity instead of relying only on exact
// declared type names.
type SQLiteMapper struct{}

func (SQLiteMapper) Map(column Column) (string, bool) {
	for _, candidate := range typeCandidates(column) {
		value := normalizeType(candidate)
		upper := strings.ToUpper(value)
		switch {
		case strings.TrimSpace(upper) == "UUID":
			return "uuid.UUID", true
		case strings.Contains(upper, "BOOLEAN"), strings.Contains(upper, "BOOL"):
			return "bool", true
		case strings.Contains(upper, "DATE"), strings.Contains(upper, "DATETIME"), strings.Contains(upper, "TIMESTAMP"):
			return "time.Time", true
		case strings.Contains(upper, "JSON"):
			return "datatypes.JSON", true
		case strings.Contains(upper, "INT"):
			return "int64", true
		case strings.Contains(upper, "CHAR"), strings.Contains(upper, "CLOB"), strings.Contains(upper, "TEXT"):
			return "string", true
		case strings.Contains(upper, "REAL"), strings.Contains(upper, "FLOA"), strings.Contains(upper, "DOUB"):
			return "float64", true
		case strings.Contains(upper, "BLOB"), strings.Contains(upper, "BINARY"), strings.Contains(upper, "VARBINARY"):
			return "[]byte", true
		case strings.Contains(upper, "NUMERIC"), strings.Contains(upper, "DECIMAL"):
			return "decimal.Decimal", true
		}
	}
	return "", false
}
