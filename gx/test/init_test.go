package test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanechi/gonex/gx/internal/gen"
)

func TestInitProjectExtractsAndRewritesProjectIdentifiers(t *testing.T) {
	server := archiveServer(t, demoArchive(t))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "app")
	result, err := gen.InitProject(target, gen.InitOptions{ModulePath: "example.com/app", Name: "app", TemplateURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) == 0 {
		t.Fatal("expected initialization changes")
	}
	content, err := os.ReadFile(filepath.Join(target, "internal/cmd/root.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "example.com/app") || !strings.Contains(string(content), "app") {
		t.Fatalf("project identifiers were not rewritten: %s", content)
	}
	goMod, _ := os.ReadFile(filepath.Join(target, "go.mod"))
	if strings.Contains(string(goMod), "replace github.com/lanechi/gonex") {
		t.Fatalf("repository-local replace remained in generated go.mod: %s", goMod)
	}
}

func TestInitProjectDryRunAndValidation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	result, err := gen.InitProject(target, gen.InitOptions{ModulePath: "example.com/app", TemplateURL: "http://invalid.test/template", DryRun: true})
	if err != nil || len(result.Changes) == 0 {
		t.Fatalf("dry-run result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote target: %v", err)
	}
	if _, err := gen.InitProject(filepath.Join(t.TempDir(), "app"), gen.InitOptions{ModulePath: "bad module"}); err == nil {
		t.Fatal("invalid module path was accepted")
	}
}

func TestInitProjectValidationFailureDoesNotPublishTarget(t *testing.T) {
	server := archiveServer(t, []byte("not a gzip archive"))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "app")
	_, err := gen.InitProject(target, gen.InitOptions{ModulePath: "example.com/app", TemplateURL: server.URL})
	if err == nil {
		t.Fatal("invalid template archive was accepted")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("invalid template published target: %v", statErr)
	}
}

func archiveServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/gzip")
		_, _ = writer.Write(body)
	}))
}

func demoArchive(t *testing.T) []byte {
	t.Helper()
	files := map[string]string{
		"go.mod":  "module github.com/lanechi/gonex/examples/demo\n\nrequire github.com/lanechi/gonex v0.0.0\n\nreplace github.com/lanechi/gonex => ../..\n",
		"main.go": "package main\n", "README.md": "# demo\n", "AGENTS.md": "demo\n", ".env.example": "DATABASE_DSN=\n", ".gitignore": ".env\n",
		".codex/config.toml":           "[agents]\nenabled = true\n",
		".codex/agents/architect.toml": "name = \"architect\"\n", ".codex/agents/worker.toml": "name = \"worker\"\n", ".codex/agents/reviewer.toml": "name = \"reviewer\"\n", ".codex/agents/explorer.toml": "name = \"explorer\"\n", ".codex/agents/tester.toml": "name = \"tester\"\n",
		".agents/skills/gonex-create-resource/SKILL.md": "---\nname: create\n---\n", ".agents/skills/gonex-design-api/SKILL.md": "---\nname: api\n---\n", ".agents/skills/gonex-implement-controller/SKILL.md": "---\nname: controller\n---\n", ".agents/skills/gonex-implement-service/SKILL.md": "---\nname: service\n---\n", ".agents/skills/gonex-review-project/SKILL.md": "---\nname: review\n---\n",
		"api/hello/hello.go": "package hello\n", "api/hello/v1/hello.go": "package v1\n", "internal/database/database.go": "package database\nimport _ \"gorm.io/driver/postgres\"\n", "internal/cmd/cmd.go": "package cmd\n", "internal/cmd/root.go": "package cmd // github.com/lanechi/gonex/examples/demo gonex-demo\n", "internal/controller/hello/hello.go": "package hello\n", "internal/controller/hello/hello_new.go": "package hello\n", "internal/controller/hello/hello_v1_hello.go": "package hello\n", "internal/logic/hello/hello.go": "package hello\n", "internal/service/hello.go": "package service\n",
	}
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	for name, content := range files {
		header := &tar.Header{Name: "root/examples/demo/" + name, Mode: 0o644, Size: int64(len(content))}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
