package dao

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

func ensureModelDependencies(project Project, result *Result) error {
	path := project.Resolve("go.mod")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project go.mod: %w", err)
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return fmt.Errorf("parse project go.mod: %w", err)
	}
	for _, dependency := range []struct {
		path    string
		version string
	}{
		{path: "gorm.io/gen", version: "v0.3.28"},
		{path: "gorm.io/plugin/dbresolver", version: "v1.5.3"},
		{path: "github.com/google/uuid", version: "v1.6.0"},
		{path: "github.com/shopspring/decimal", version: "v1.4.0"},
		{path: "gorm.io/datatypes", version: "v1.2.4"},
	} {
		file.AddRequire(dependency.path, dependency.version)
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

func runGoModTidy(projectRoot string) error {
	command := exec.Command("go", "mod", "tidy")
	command.Dir = projectRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
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
