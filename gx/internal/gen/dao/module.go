package dao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

const goModTidyTimeout = 2 * time.Minute

func ensureModelDependencies(project Project, result *Result, driver string) error {
	path := project.Resolve("go.mod")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project go.mod: %w", err)
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return fmt.Errorf("parse project go.mod: %w", err)
	}

	dependencies := []struct {
		path    string
		version string
	}{
		{path: "gorm.io/gen", version: "v0.3.28"},
		{path: "gorm.io/plugin/dbresolver", version: "v1.6.2"},
		{path: "github.com/shopspring/decimal", version: "v1.4.0"},
		{path: "gorm.io/datatypes", version: "v1.2.4"},
	}
	if isPostgresDriver(driver) {
		// pgx 5.11 is the first release that uses Go 1.27's
		// driver.RowsColumnScanner support for direct PostgreSQL scans. Generated
		// PostgreSQL array serializers also delegate array encoding and decoding
		// to pgx, so keep the project on at least this version.
		dependencies = append(dependencies, struct {
			path    string
			version string
		}{path: "github.com/jackc/pgx/v5", version: "v5.11.0"})
	}
	for _, dependency := range dependencies {
		addMinimumRequire(file, dependency.path, dependency.version)
	}

	updated, err := file.Format()
	if err != nil {
		return fmt.Errorf("format project go.mod: %w", err)
	}
	if bytesEqual(content, updated) {
		return nil
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("update project go.mod: %w", err)
	}
	result.Add("UPDATE", "go.mod", "added GORM model generation dependencies")
	return nil
}

func addMinimumRequire(file *modfile.File, path, minimum string) {
	for _, requirement := range file.Require {
		if requirement.Mod.Path != path {
			continue
		}
		if semver.Compare(requirement.Mod.Version, minimum) >= 0 {
			return
		}
		break
	}
	file.AddRequire(path, minimum)
}

func runGoModTidy(projectRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), goModTidyTimeout)
	defer cancel()
	// DAO generation validates its staged Go sources before publication. Use
	// tidy's error-tolerant mode here so unrelated, temporarily broken project
	// packages do not prevent those already-validated files from being
	// published. Module-file errors and other fatal tidy failures still surface.
	command := exec.CommandContext(ctx, "go", "mod", "tidy", "-e")
	command.Dir = projectRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("run go mod tidy: %w", ctxErr)
		}
		return fmt.Errorf("run go mod tidy: %w", err)
	}
	return nil
}

func snapshotModuleFiles(projectRoot string) (map[string][]byte, error) {
	return snapshotFiles(
		projectRoot,
		filepath.Join(projectRoot, "go.mod"),
		filepath.Join(projectRoot, "go.sum"),
	)
}

func restoreModuleFiles(projectRoot string, snapshot map[string][]byte) error {
	var restoreErrors []error
	for _, relative := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(projectRoot, relative)
		content, existed := snapshot[relative]
		if !existed {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove generated %s: %w", relative, err))
			}
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", relative, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func addModuleFileChanges(result *Result, before, after map[string][]byte) {
	paths := map[string]struct{}{
		"go.mod": {},
		"go.sum": {},
	}
	for path := range paths {
		beforeContent, existedBefore := before[path]
		afterContent, existsAfter := after[path]
		if existedBefore && existsAfter && bytesEqual(beforeContent, afterContent) {
			continue
		}
		kind := "UPDATE"
		switch {
		case !existedBefore && existsAfter:
			kind = "CREATE"
		case existedBefore && !existsAfter:
			kind = "DELETE"
		case !existedBefore && !existsAfter:
			continue
		}
		if hasChangePath(*result, path) {
			continue
		}
		result.Add(kind, path, "go mod tidy")
	}
}

func hasChangePath(result Result, path string) bool {
	for _, change := range result.Changes {
		if change.Path == path {
			return true
		}
	}
	return false
}
