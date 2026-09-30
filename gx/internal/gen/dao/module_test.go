package dao

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestEnsureModelDependenciesUsesCompatibleDBResolver(t *testing.T) {
	root := t.TempDir()
	goModPath := filepath.Join(root, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.27.0\n\nrequire gorm.io/gorm v1.31.2\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	var result Result
	if err := ensureModelDependencies(Project{Root: root}, &result, "sqlite"); err != nil {
		t.Fatalf("ensure model dependencies: %v", err)
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	file, err := modfile.Parse(goModPath, content, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}

	requirements := make(map[string]string, len(file.Require))
	for _, requirement := range file.Require {
		requirements[requirement.Mod.Path] = requirement.Mod.Version
	}
	if got := requirements["gorm.io/plugin/dbresolver"]; got != "v1.6.2" {
		t.Fatalf("dbresolver version = %q, want v1.6.2", got)
	}
	if got := requirements["gorm.io/gorm"]; got != "v1.31.2" {
		t.Fatalf("gorm version = %q, want existing v1.31.2 preserved", got)
	}
	if _, exists := requirements["github.com/jackc/pgx/v5"]; exists {
		t.Fatal("non-PostgreSQL generation unexpectedly added pgx")
	}
}

func TestEnsureModelDependenciesRequiresPGX511ForPostgres(t *testing.T) {
	root := t.TempDir()
	goModPath := filepath.Join(root, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module example.com/app\n\ngo 1.27.0\n\nrequire github.com/jackc/pgx/v5 v5.10.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	var result Result
	if err := ensureModelDependencies(Project{Root: root}, &result, "postgres"); err != nil {
		t.Fatalf("ensure model dependencies: %v", err)
	}

	content, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	file, err := modfile.Parse(goModPath, content, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	for _, requirement := range file.Require {
		if requirement.Mod.Path == "github.com/jackc/pgx/v5" {
			if requirement.Mod.Version != "v5.11.0" {
				t.Fatalf("pgx version = %q, want v5.11.0", requirement.Mod.Version)
			}
			return
		}
	}
	t.Fatal("PostgreSQL generation did not add pgx")
}

func TestAddMinimumRequireDoesNotDowngrade(t *testing.T) {
	file, err := modfile.Parse("go.mod", []byte("module example.com/app\n\ngo 1.27.0\n\nrequire example.com/dependency v1.3.0\n"), nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	addMinimumRequire(file, "example.com/dependency", "v1.2.0")
	for _, requirement := range file.Require {
		if requirement.Mod.Path == "example.com/dependency" {
			if requirement.Mod.Version != "v1.3.0" {
				t.Fatalf("dependency was downgraded to %q", requirement.Mod.Version)
			}
			return
		}
	}
	t.Fatal("dependency disappeared")
}

func TestRunGoModTidyIgnoresUnrelatedPackageLoadErrors(t *testing.T) {
	t.Setenv("GOPROXY", "off")
	root := t.TempDir()
	writeDAOTestFile(t, filepath.Join(root, "go.mod"), "module example.com/broken\n\ngo 1.27.0\n")
	writeDAOTestFile(t, filepath.Join(root, "broken.go"), `package broken

import _ "example.com/broken/missing"
`)

	if err := runGoModTidy(root); err != nil {
		t.Fatalf("go mod tidy -e rejected an unrelated package load error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod missing after tolerant tidy: %v", err)
	}
}
