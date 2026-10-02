package test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type postgresArrayBindingModel struct {
	ID   int64                `gorm:"column:id;primaryKey"`
	Tags pgtype.Array[string] `gorm:"column:tags;type:text[]"`
}

func (postgresArrayBindingModel) TableName() string { return "array_values" }

func TestGORMBindsPGXArrayAsSingleParameter(t *testing.T) {
	database, err := gorm.Open(
		postgres.Open("postgres://postgres:postgres@127.0.0.1:1/test?sslmode=disable"),
		&gorm.Config{DisableAutomaticPing: true, DryRun: true},
	)
	if err != nil {
		t.Fatalf("open dry-run PostgreSQL database: %v", err)
	}

	tags := pgtype.Array[string]{
		Elements: []string{"protest", "labor"},
		Dims:     []pgtype.ArrayDimension{{Length: 2, LowerBound: 1}},
		Valid:    true,
	}
	statement := database.Create(&postgresArrayBindingModel{ID: 1, Tags: tags}).Statement
	if statement.Error != nil {
		t.Fatalf("build create statement: %v", statement.Error)
	}
	if len(statement.Vars) != 2 {
		t.Fatalf("GORM bound %d variables, want id plus one array variable; SQL=%s vars=%#v", len(statement.Vars), statement.SQL.String(), statement.Vars)
	}
	if !reflect.DeepEqual(statement.Vars[1], tags) {
		t.Fatalf("array variable = %#v, want %#v", statement.Vars[1], tags)
	}
	if strings.Contains(statement.SQL.String(), "($2,$3)") {
		t.Fatalf("GORM expanded pgtype.Array as a SQL value list: %s", statement.SQL.String())
	}
}

func TestGORMExpandsPlainSliceInsteadOfBindingPostgresArray(t *testing.T) {
	type plainSliceModel struct {
		ID   int64    `gorm:"column:id;primaryKey"`
		Tags []string `gorm:"column:tags;type:text[]"`
	}
	database, err := gorm.Open(
		postgres.Open("postgres://postgres:postgres@127.0.0.1:1/test?sslmode=disable"),
		&gorm.Config{DisableAutomaticPing: true, DryRun: true},
	)
	if err != nil {
		t.Fatalf("open dry-run PostgreSQL database: %v", err)
	}

	statement := database.Table("array_values").Create(&plainSliceModel{ID: 1, Tags: []string{"a", "b"}}).Statement
	if statement.Error != nil {
		t.Fatalf("build plain-slice create statement: %v", statement.Error)
	}
	if len(statement.Vars) != 3 {
		t.Fatalf("plain []string unexpectedly bound as one PostgreSQL array variable: SQL=%s vars=%#v", statement.SQL.String(), statement.Vars)
	}
}
