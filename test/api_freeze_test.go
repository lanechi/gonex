package ghttp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemovedCompatibilitySurfaceDoesNotReturn protects deliberate removals
// without freezing the pre-v1 public API. Gonex intentionally allows breaking
// API cleanup before v1; compatibility aliases and forwarding shims must not be
// reintroduced to preserve obsolete surfaces.
func TestRemovedCompatibilitySurfaceDoesNotReturn(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		for _, forbidden := range []string{
			"NewRedisStorage",
			"NewOwnedRedisStorage",
			"type RedisStorage",
			"addressSet",
			"openapiSet",
			"modeSet",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("removed compatibility surface %q remains in %s", forbidden, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenAPIStagesRemainSplit(t *testing.T) {
	root := filepath.Join("..")
	for _, path := range []string{
		filepath.Join(root, "openapi", "parameters.go"),
		filepath.Join(root, "openapi", "request_body.go"),
		filepath.Join(root, "openapi", "responses.go"),
		filepath.Join(root, "openapi", "security.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required OpenAPI stage file missing: %s: %v", path, err)
		}
	}
}
