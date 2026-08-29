package ghttp_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	redisImportPath = "github.com/redis/go-redis/v9"
	gonexImportRoot = "github.com/lanechi/gonex/"
)

func TestCorePackageDependencyBoundaries(t *testing.T) {
	root := filepath.Join("..")
	forbidden := map[string][]string{
		"router":    {"github.com/lanechi/gonex/ghttp"},
		"config":    {"github.com/lanechi/gonex/ghttp"},
		"logging":   {"github.com/lanechi/gonex/ghttp"},
		"session":   {"github.com/lanechi/gonex/ghttp"},
		"openapi":   {"github.com/lanechi/gonex/ghttp"},
		"lifecycle": {"github.com/lanechi/gonex/ghttp"},
		"scheduler": {"github.com/lanechi/gonex/ghttp"},
	}
	for packageName, blocked := range forbidden {
		packageDir := filepath.Join(root, packageName)
		err := filepath.Walk(packageDir, func(path string, infoFilePath os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if infoFilePath.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, importSpec := range file.Imports {
				importPath := strings.Trim(importSpec.Path.Value, `"`)
				for _, blockedPath := range blocked {
					if importPath == blockedPath {
						t.Errorf("%s imports forbidden package %s", path, blockedPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", packageName, err)
		}
	}
}

func TestCorePackageLocalDependencyAllowlist(t *testing.T) {
	root := filepath.Join("..")
	allowed := map[string]map[string]struct{}{
		"router":    {},
		"config":    {},
		"logging":   {},
		"session":   {},
		"openapi":   {"github.com/lanechi/gonex/router": {}},
		"lifecycle": {},
		"scheduler": {"github.com/lanechi/gonex/logging": {}},
	}

	for packageName, packageAllowlist := range allowed {
		packageDir := filepath.Join(root, packageName)
		err := filepath.Walk(packageDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, importSpec := range file.Imports {
				importPath := strings.Trim(importSpec.Path.Value, `"`)
				if !strings.HasPrefix(importPath, gonexImportRoot) {
					continue
				}
				if _, ok := packageAllowlist[importPath]; !ok {
					t.Errorf("%s imports unreviewed Gonex package %s", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", packageName, err)
		}
	}
}

func TestCoreRejectsRedisSessionDependencies(t *testing.T) {
	root := filepath.Join("..")
	paths, err := coreProductionGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, `"`)
			if importPath == redisImportPath && filepath.ToSlash(relative) != "contrib/redislog/redis.go" {
				t.Errorf("%s imports go-redis outside contrib/redislog", path)
			}
			lowerImportPath := strings.ToLower(importPath)
			if strings.Contains(lowerImportPath, "redis") && strings.Contains(lowerImportPath, "session") {
				t.Errorf("%s imports forbidden Redis Session package %s", path, importPath)
			}
		}
	}
}

func coreProductionGoFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch filepath.ToSlash(relative) {
			case ".git", "benchmarks", "examples", "gx", "test":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk core production Go files: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}
