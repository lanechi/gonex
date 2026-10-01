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

func TestPostgresArrayMappingUsesDedicatedTypesWhenAvailable(t *testing.T) {
	tests := []struct {
		dataType string
		want     string
	}{
		{"text[]", "pq.StringArray"},
		{"bigint[]", "pq.Int64Array"},
		{"uuid[]", "[]datatypes.UUID"},
		{"bytea[]", "pq.ByteaArray"},
		{"interval[]", "[]pgtype.Interval"},
		{"int4range[]", "[]pgtype.Range[int32]"},
		{"_float8", "pq.Float64Array"},
	}
	for _, test := range tests {
		t.Run(test.dataType, func(t *testing.T) {
			got := typemapping.MapFieldType(typemapping.DatabasePostgres, typemapping.Column{
				DataType: test.dataType,
				Nullable: true,
			})
			if got != test.want {
				t.Fatalf("MapFieldType(%q) = %q, want %q", test.dataType, got, test.want)
			}
		})
	}
}

func TestTypeMappingPreservesNullableCollectionValues(t *testing.T) {
	if got := typemapping.MapFieldType(typemapping.DatabasePostgres, typemapping.Column{DataType: "text[]", Nullable: true}); got != "pq.StringArray" {
		t.Fatalf("nullable text array = %q, want pq.StringArray", got)
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
		Columns: []gorm.ColumnType{
			testColumnType{name: "uuid", databaseType: "uuid", columnType: "uuid"},
			testColumnType{name: "amount", databaseType: "numeric", columnType: "numeric(20,8)"},
			testColumnType{name: "duration", databaseType: "interval", columnType: "interval"},
			testColumnType{name: "ip", databaseType: "inet", columnType: "inet"},
			testColumnType{name: "mac", databaseType: "macaddr", columnType: "macaddr"},
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

func TestTypeMappingBuildsWarningsAndColumnMetadata(t *testing.T) {
	mapping := typemapping.BuildDataTypeMap(typemapping.DatabasePostgres, []typemapping.TableColumns{{
		Table: "users",
		Columns: []gorm.ColumnType{
			testColumnType{name: "id", databaseType: "int8", columnType: "bigint"},
			testColumnType{name: "location", databaseType: "USER-DEFINED", columnType: "geometry"},
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
