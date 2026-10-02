package test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	typemapping "github.com/lanechi/gonex/gx/internal/type_mapping"
	"gorm.io/gorm"
)

type testColumnType struct {
	name, databaseType, columnType, defaultValue string
	nullable                                     bool
}

func (column testColumnType) Name() string             { return column.name }
func (column testColumnType) DatabaseTypeName() string { return column.databaseType }
func (column testColumnType) ColumnType() (string, bool) {
	return column.columnType, column.columnType != ""
}
func (column testColumnType) PrimaryKey() (bool, bool)    { return false, true }
func (column testColumnType) AutoIncrement() (bool, bool) { return false, true }
func (column testColumnType) Nullable() (bool, bool)      { return column.nullable, true }
func (column testColumnType) Unique() (bool, bool)        { return false, true }
func (column testColumnType) DefaultValue() (string, bool) {
	return column.defaultValue, column.defaultValue != ""
}
func (column testColumnType) Comment() (string, bool)           { return "", false }
func (column testColumnType) ScanType() reflect.Type            { return nil }
func (column testColumnType) Length() (int64, bool)             { return 0, false }
func (column testColumnType) DecimalSize() (int64, int64, bool) { return 0, 0, false }

var _ gorm.ColumnType = testColumnType{}

func postgresArrayColumn(name string, arrayOID uint32, arrayName string, elementOID uint32, elementName string) typemapping.Column {
	return typemapping.Column{
		Name:       name,
		DataType:   "ARRAY",
		ColumnType: elementName + "[]",
		Nullable:   true,
		Postgres: &typemapping.PostgresType{
			OID:         arrayOID,
			Name:        arrayName,
			IsArray:     true,
			ElementOID:  elementOID,
			ElementName: elementName,
		},
	}
}

func TestTypeMappingAcrossDatabases(t *testing.T) {
	tests := []struct {
		name   string
		driver typemapping.DatabaseType
		column typemapping.Column
		want   string
	}{
		{"postgres uuid", typemapping.DatabasePostgres, typemapping.Column{DataType: "uuid", Nullable: true}, "*datatypes.UUID"},
		{"postgres jsonb", typemapping.DatabasePostgres, typemapping.Column{DataType: "jsonb"}, "datatypes.JSON"},
		{"postgres date", typemapping.DatabasePostgres, typemapping.Column{DataType: "date", Nullable: true}, "*datatypes.Date"},
		{"postgres time", typemapping.DatabasePostgres, typemapping.Column{DataType: "time", Nullable: true}, "pgtype.Time"},
		{"postgres interval", typemapping.DatabasePostgres, typemapping.Column{DataType: "interval", Nullable: true}, "pgtype.Interval"},
		{"postgres inet", typemapping.DatabasePostgres, typemapping.Column{DataType: "inet", Nullable: true}, "*netip.Addr"},
		{"postgres cidr", typemapping.DatabasePostgres, typemapping.Column{DataType: "cidr"}, "netip.Prefix"},
		{"postgres macaddr", typemapping.DatabasePostgres, typemapping.Column{DataType: "macaddr"}, "net.HardwareAddr"},
		{"postgres bits", typemapping.DatabasePostgres, typemapping.Column{DataType: "varbit", Nullable: true}, "pgtype.Bits"},
		{"postgres point", typemapping.DatabasePostgres, typemapping.Column{DataType: "point"}, "pgtype.Point"},
		{"postgres hstore", typemapping.DatabasePostgres, typemapping.Column{DataType: "hstore"}, "pgtype.Hstore"},
		{"postgres tsvector", typemapping.DatabasePostgres, typemapping.Column{DataType: "tsvector"}, "pgtype.TSVector"},
		{"postgres int4 range", typemapping.DatabasePostgres, typemapping.Column{DataType: "int4range"}, "pgtype.Range[int32]"},
		{"postgres numeric multirange", typemapping.DatabasePostgres, typemapping.Column{DataType: "nummultirange"}, "pgtype.Multirange[pgtype.Range[pgtype.Numeric]]"},
		{"postgres money", typemapping.DatabasePostgres, typemapping.Column{DataType: "money"}, "string"},
		{"postgres timetz", typemapping.DatabasePostgres, typemapping.Column{DataType: "timetz"}, "string"},
		{"mysql bool", typemapping.DatabaseMySQL, typemapping.Column{DataType: "tinyint", ColumnType: "tinyint(1)"}, "bool"},
		{"mysql decimal", typemapping.DatabaseMySQL, typemapping.Column{DataType: "decimal", ColumnType: "decimal(20,8)"}, "decimal.Decimal"},
		{"sqlite integer", typemapping.DatabaseSQLite, typemapping.Column{DataType: "BIGINT"}, "int64"},
		{"sqlite json", typemapping.DatabaseSQLite, typemapping.Column{DataType: "JSON"}, "datatypes.JSON"},
		{"sqlite uuid", typemapping.DatabaseSQLite, typemapping.Column{DataType: "UUID"}, "datatypes.UUID"},
		{"sqlserver uuid", typemapping.DatabaseSQLServer, typemapping.Column{DataType: "uniqueidentifier"}, "datatypes.UUID"},
		{"sqlserver max", typemapping.DatabaseSQLServer, typemapping.Column{DataType: "nvarchar", ColumnType: "nvarchar(max)"}, "string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := typemapping.MapFieldType(test.driver, test.column); got != test.want {
				t.Fatalf("MapFieldType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPostgresArrayMappingUsesNativeSlices(t *testing.T) {
	tests := []struct {
		name        string
		arrayOID    uint32
		arrayName   string
		elementOID  uint32
		elementName string
		want        string
	}{
		{"text", 1009, "_text", 25, "text", "[]string"},
		{"bigint", 1016, "_int8", 20, "int8", "[]int64"},
		{"uuid", 2951, "_uuid", 2950, "uuid", "[]datatypes.UUID"},
		{"bytea", 1001, "_bytea", 17, "bytea", "[][]byte"},
		{"interval", 1187, "_interval", 1186, "interval", "[]pgtype.Interval"},
		{"int4range", 3905, "_int4range", 3904, "int4range", "[]pgtype.Range[int32]"},
		{"float8", 1022, "_float8", 701, "float8", "[]float64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := typemapping.MapFieldType(
				typemapping.DatabasePostgres,
				postgresArrayColumn("values", test.arrayOID, test.arrayName, test.elementOID, test.elementName),
			)
			if got != test.want {
				t.Fatalf("MapFieldType(%q) = %q, want %q", test.elementName, got, test.want)
			}
		})
	}
}

func TestPostgresArrayMappingRequiresCatalogMetadata(t *testing.T) {
	for _, dataType := range []string{"text[]", "_text", "ARRAY"} {
		got := typemapping.MapFieldType(typemapping.DatabasePostgres, typemapping.Column{
			DataType: dataType,
			Nullable: true,
		})
		if got != "*string" {
			t.Fatalf("MapFieldType(%q) without catalog metadata = %q, want *string fallback", dataType, got)
		}
	}
}

func TestPostgresArraysUseColumnSpecificSerializerMapping(t *testing.T) {
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabasePostgres, []typemapping.TableColumns{{
		Table: "public.documents",
		Columns: []typemapping.Column{
			func() typemapping.Column {
				column := postgresArrayColumn("source_urls", 1009, "_text", 25, "text")
				column.TableName = "public.documents"
				return column
			}(),
			func() typemapping.Column {
				column := postgresArrayColumn("related_ids", 1016, "_int8", 20, "int8")
				column.TableName = "public.documents"
				return column
			}(),
			{TableName: "public.documents", Name: "title", DataType: "text", ColumnType: "text"},
		},
	}})

	if _, exists := mapping.TypeMap["ARRAY"]; exists {
		t.Fatal("PostgreSQL ARRAY must not use a global GORM Gen data type hook")
	}
	source := mapping.Fields["public.documents"]["source_urls"]
	if source.Type != "[]string" || source.Serializer != "pgarray" || source.PostgresDBType != "_text" {
		t.Fatalf("unexpected source_urls mapping: %#v", source)
	}
	related := mapping.Fields["public.documents"]["related_ids"]
	if related.Type != "[]int64" || related.Serializer != "pgarray" || related.PostgresDBType != "_int8" {
		t.Fatalf("unexpected related_ids mapping: %#v", related)
	}
	if got := mapping.TypeMap["text"](testColumnType{databaseType: "text", columnType: "text"}); got != "string" {
		t.Fatalf("text scalar mapping = %q, want string", got)
	}
}

func TestPostgresExtensionArrayIsNotAutoSerialized(t *testing.T) {
	column := postgresArrayColumn("labels", 16391, "_citext", 16390, "citext")
	column.TableName = "public.documents"
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabasePostgres, []typemapping.TableColumns{{
		Table:   "public.documents",
		Columns: []typemapping.Column{column},
	}})

	if fields := mapping.Fields["public.documents"]; len(fields) != 0 {
		t.Fatalf("unsupported extension array unexpectedly received serializer mapping: %#v", fields)
	}
	if len(mapping.Warnings) != 1 || mapping.Warnings[0].Column != "labels" {
		t.Fatalf("unexpected extension array warnings: %#v", mapping.Warnings)
	}
}

func TestTypeMappingPreservesNullableCollectionValues(t *testing.T) {
	array := postgresArrayColumn("tags", 1009, "_text", 25, "text")
	if got := typemapping.MapFieldType(typemapping.DatabasePostgres, array); got != "[]string" {
		t.Fatalf("nullable text array = %q, want []string", got)
	}
	if got := typemapping.MapFieldType(typemapping.DatabasePostgres, typemapping.Column{DataType: "bigint", Nullable: true}); got != "*int64" {
		t.Fatalf("nullable bigint = %q, want *int64", got)
	}
	if got := typemapping.MapFieldType(typemapping.DatabasePostgres, typemapping.Column{DataType: "interval", Nullable: true}); got != "pgtype.Interval" {
		t.Fatalf("nullable interval = %q, want pgtype.Interval", got)
	}
}

func TestTypeMappingCollectsExternalImports(t *testing.T) {
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabasePostgres, []typemapping.TableColumns{{
		Table: "all_types",
		Columns: []typemapping.Column{
			{Name: "uuid", DataType: "uuid", ColumnType: "uuid"},
			{Name: "amount", DataType: "numeric", ColumnType: "numeric(20,8)"},
			{Name: "duration", DataType: "interval", ColumnType: "interval"},
			{Name: "ip", DataType: "inet", ColumnType: "inet"},
			{Name: "mac", DataType: "macaddr", ColumnType: "macaddr"},
		},
	}})
	for _, want := range []string{
		"github.com/jackc/pgx/v5/pgtype",
		"github.com/shopspring/decimal",
		"gorm.io/datatypes",
		"net",
		"net/netip",
	} {
		if !slices.Contains(mapping.Imports, want) {
			t.Fatalf("imports = %#v, missing %q", mapping.Imports, want)
		}
	}
}

func TestTypeMappingKeepsScalarHooksColumnSensitive(t *testing.T) {
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabaseMySQL, []typemapping.TableColumns{{
		Table: "values",
		Columns: []typemapping.Column{
			{Name: "enabled", DataType: "tinyint", ColumnType: "tinyint(1)"},
		},
	}})
	hook := mapping.TypeMap["tinyint"]
	if hook == nil {
		t.Fatal("missing tinyint type hook")
	}
	if got := hook(testColumnType{databaseType: "tinyint", columnType: "tinyint(1)"}); got != "bool" {
		t.Fatalf("tinyint(1) = %q, want bool", got)
	}
	if got := hook(testColumnType{databaseType: "tinyint", columnType: "tinyint(4)"}); got != "int8" {
		t.Fatalf("tinyint(4) = %q, want int8", got)
	}
}

func TestTypeMappingBuildsWarningsAndColumnMetadata(t *testing.T) {
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabasePostgres, []typemapping.TableColumns{{
		Table: "users",
		Columns: []typemapping.Column{
			{Name: "id", DataType: "int8", ColumnType: "bigint"},
			{Name: "location", DataType: "USER-DEFINED", ColumnType: "geometry"},
		},
	}})
	if got := mapping.TypeMap["int8"](testColumnType{databaseType: "int8", columnType: "bigint"}); got != "int64" {
		t.Fatalf("mapped hook type = %q, want int64", got)
	}
	if len(mapping.Warnings) != 1 || !strings.Contains(mapping.Warnings[0].String(), "table=users column=location") {
		t.Fatalf("unexpected warnings: %#v", mapping.Warnings)
	}
	column := typemapping.ColumnFromGORM("users", testColumnType{name: "amount", databaseType: "decimal", columnType: "decimal(10,2)", nullable: true, defaultValue: "0"})
	if column.TableName != "users" || column.Name != "amount" || !column.Nullable || !column.HasDefault {
		t.Fatalf("unexpected column metadata: %#v", column)
	}
}
