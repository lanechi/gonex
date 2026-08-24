package typemapping

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// DatabaseType identifies the database dialect used for schema introspection.
type DatabaseType string

const (
	DatabasePostgres  DatabaseType = "postgres"
	DatabaseMySQL     DatabaseType = "mysql"
	DatabaseSQLite    DatabaseType = "sqlite"
	DatabaseSQLServer DatabaseType = "sqlserver"
)

// Column is the database metadata needed for field type mapping.
type Column struct {
	TableName  string
	Name       string
	DataType   string
	ColumnType string
	Nullable   bool
	HasDefault bool
	Length     int64
	Precision  int64
	Scale      int64
	Unsigned   bool
}

// Mapper maps one database column to a Go field type. The returned type does
// not include nullable decoration; GORM Gen applies its existing nullable
// strategy to generated fields.
type Mapper interface {
	Map(column Column) (string, bool)
}

// TableColumns groups introspected columns by table for mapping and warnings.
type TableColumns struct {
	Table   string
	Columns []gorm.ColumnType
}

// Warning describes a database type that was not recognized by a mapper.
type Warning struct {
	Driver     DatabaseType
	Table      string
	Column     string
	DataType   string
	ColumnType string
}

func (warning Warning) String() string {
	return fmt.Sprintf("unsupported database column type: driver=%s table=%s column=%s dataType=%s columnType=%s", warning.Driver, warning.Table, warning.Column, warning.DataType, warning.ColumnType)
}

// Mapping contains the GORM Gen hook and the imports needed by mapped types.
type Mapping struct {
	TypeMap  map[string]func(gorm.ColumnType) string
	Imports  []string
	Warnings []Warning
}

// New returns the mapper for a supported database driver.
func New(driver DatabaseType) Mapper {
	switch normalizeDriver(driver) {
	case DatabasePostgres:
		return PostgresMapper{}
	case DatabaseMySQL:
		return MySQLMapper{}
	case DatabaseSQLite:
		return SQLiteMapper{}
	case DatabaseSQLServer:
		return SQLServerMapper{}
	default:
		return unsupportedMapper{}
	}
}

// MapFieldType maps a column and applies the nullable pointer strategy for
// scalar values. Slice and JSON values intentionally remain non-pointer.
func MapFieldType(driver DatabaseType, column Column) string {
	mapper := New(driver)
	fieldType, ok := mapper.Map(column)
	if !ok || strings.TrimSpace(fieldType) == "" {
		fieldType = "string"
	}
	return applyNullable(column, fieldType)
}

// BuildDataTypeMap creates the exact DatabaseTypeName hooks expected by GORM
// Gen. GORM Gen keys its hook map by the driver's raw type name, so the keys
// are collected from the current schema rather than guessed from a fixed list.
func BuildDataTypeMap(driver DatabaseType, tables []TableColumns) Mapping {
	mapper := New(driver)
	result := Mapping{TypeMap: make(map[string]func(gorm.ColumnType) string)}
	importSet := make(map[string]struct{})

	for _, table := range tables {
		for _, columnType := range table.Columns {
			column := ColumnFromGORM(table.Table, columnType)
			key := strings.TrimSpace(columnType.DatabaseTypeName())
			if key != "" {
				if _, exists := result.TypeMap[key]; !exists {
					result.TypeMap[key] = func(current gorm.ColumnType) string {
						mapped, ok := mapper.Map(ColumnFromGORM("", current))
						if !ok || strings.TrimSpace(mapped) == "" {
							return "string"
						}
						return mapped
					}
				}
			}

			mapped, ok := mapper.Map(column)
			if !ok {
				result.Warnings = append(result.Warnings, Warning{
					Driver:     normalizeDriver(driver),
					Table:      table.Table,
					Column:     column.Name,
					DataType:   column.DataType,
					ColumnType: column.ColumnType,
				})
				continue
			}
			for _, importPath := range importsForType(mapped) {
				importSet[importPath] = struct{}{}
			}
		}
	}

	for importPath := range importSet {
		result.Imports = append(result.Imports, importPath)
	}
	sort.Strings(result.Imports)
	sort.Slice(result.Warnings, func(left, right int) bool {
		if result.Warnings[left].Table == result.Warnings[right].Table {
			return result.Warnings[left].Column < result.Warnings[right].Column
		}
		return result.Warnings[left].Table < result.Warnings[right].Table
	})
	return result
}

// ColumnFromGORM converts GORM's introspection interface into the mapper
// metadata used by the independent database adapters.
func ColumnFromGORM(table string, columnType gorm.ColumnType) Column {
	column := Column{TableName: table}
	if columnType == nil {
		return column
	}
	column.Name = columnType.Name()
	column.DataType = columnType.DatabaseTypeName()
	column.ColumnType, _ = columnType.ColumnType()
	column.Nullable, _ = columnType.Nullable()
	column.HasDefault = hasDefault(columnType)
	column.Length, _ = columnType.Length()
	column.Precision, column.Scale, _ = columnType.DecimalSize()
	column.Unsigned = isUnsigned(column)
	return column
}

func hasDefault(columnType gorm.ColumnType) bool {
	value, ok := columnType.DefaultValue()
	return ok && strings.TrimSpace(value) != ""
}

func normalizeDriver(driver DatabaseType) DatabaseType {
	switch strings.ToLower(strings.TrimSpace(string(driver))) {
	case "postgres", "postgresql", "pgsql":
		return DatabasePostgres
	case "mysql", "mariadb", "tidb":
		return DatabaseMySQL
	case "sqlite", "sqlite3":
		return DatabaseSQLite
	case "sqlserver", "mssql":
		return DatabaseSQLServer
	default:
		return DatabaseType(strings.ToLower(strings.TrimSpace(string(driver))))
	}
}

func importsForType(fieldType string) []string {
	imports := make([]string, 0, 3)
	if strings.Contains(fieldType, "uuid.UUID") {
		imports = append(imports, "github.com/google/uuid")
	}
	if strings.Contains(fieldType, "decimal.Decimal") {
		imports = append(imports, "github.com/shopspring/decimal")
	}
	if strings.Contains(fieldType, "datatypes.JSON") {
		imports = append(imports, "gorm.io/datatypes")
	}
	return imports
}

type unsupportedMapper struct{}

func (unsupportedMapper) Map(Column) (string, bool) { return "", false }
