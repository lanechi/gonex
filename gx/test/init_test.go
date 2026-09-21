package test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
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

func TestInitProjectRejectsIncompleteSkillBundle(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
		want   string
	}{
		{
			name: "missing metadata",
			mutate: func(files map[string]string) {
				delete(files, ".agents/skills/gonex-use-dao/agents/openai.yaml")
			},
			want: ".agents/skills/gonex-use-dao/agents/openai.yaml",
		},
		{
			name: "missing reference",
			mutate: func(files map[string]string) {
				delete(files, ".agents/skills/gonex-implement-service/references/service-patterns.md")
			},
			want: ".agents/skills/gonex-implement-service/references/service-patterns.md",
		},
		{
			name: "wrong frontmatter name",
			mutate: func(files map[string]string) {
				files[".agents/skills/gonex-use-data/SKILL.md"] = "---\nname: another-skill\n---\n"
			},
			want: "must declare frontmatter name gonex-use-data",
		},
		{
			name: "default prompt does not reference skill",
			mutate: func(files map[string]string) {
				files[".agents/skills/gonex-use-config/agents/openai.yaml"] = skillMetadata("another-skill", true)
			},
			want: "default_prompt must reference $gonex-use-config",
		},
		{
			name: "implicit invocation disabled",
			mutate: func(files map[string]string) {
				files[".agents/skills/gonex-use-logging/agents/openai.yaml"] = skillMetadata("gonex-use-logging", false)
			},
			want: "must allow implicit invocation",
		},
		{
			name: "skill missing from project routing",
			mutate: func(files map[string]string) {
				files["AGENTS.md"] = strings.ReplaceAll(files["AGENTS.md"], "$gonex-use-dao", "gonex-use-dao")
			},
			want: "AGENTS.md must route tasks to $gonex-use-dao",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := archiveServer(t, demoArchiveWith(t, test.mutate))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "app")
			_, err := gen.InitProject(target, gen.InitOptions{ModulePath: "example.com/app", TemplateURL: server.URL})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want error containing %q", err, test.want)
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("invalid skill bundle published target: %v", statErr)
			}
		})
	}
}

func TestInitProjectRejectsNonGlobalConfigEntrypoint(t *testing.T) {
	server := archiveServer(t, demoArchiveWith(t, func(files map[string]string) {
		files["internal/cmd/cmd.go"] = "package cmd\n"
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "app")
	_, err := gen.InitProject(target, gen.InitOptions{ModulePath: "example.com/app", TemplateURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "global g.Cfg() configuration") {
		t.Fatalf("error=%v, want global configuration validation error", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("invalid config entrypoint published target: %v", statErr)
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
	return demoArchiveWith(t, nil)
}

func demoArchiveWith(t *testing.T, mutate func(map[string]string)) []byte {
	t.Helper()
	files := map[string]string{
		"go.mod":  "module github.com/lanechi/gonex/examples/demo\n\nrequire github.com/lanechi/gonex v0.0.0\n\nreplace github.com/lanechi/gonex => ../..\n",
		"main.go": "package main\n", "README.md": "# demo\n", "AGENTS.md": "demo\n", ".env.example": "DATABASE_DSN=\n", ".gitignore": ".env\n",
		".codex/config.toml":           "[agents]\nenabled = true\n",
		".codex/agents/architect.toml": "name = \"architect\"\n", ".codex/agents/worker.toml": "name = \"worker\"\n", ".codex/agents/reviewer.toml": "name = \"reviewer\"\n", ".codex/agents/explorer.toml": "name = \"explorer\"\n", ".codex/agents/tester.toml": "name = \"tester\"\n",
		"api/hello/hello.go": "package hello\n", "api/hello/v1/hello.go": "package v1\n", "internal/bootstrap/db/postgres.go": "package db\nimport _ \"gorm.io/driver/postgres\"\n", "internal/bootstrap/db/mysql.go": "package db\n", "internal/bootstrap/db/sqlite.go": "package db\n", "internal/cmd/cmd.go": "package cmd\nimport \"github.com/lanechi/gonex/g\"\nfunc configEntry() { _ = g.Cfg() }\n", "internal/cmd/root.go": "package cmd // github.com/lanechi/gonex/examples/demo gonex-demo\n", "internal/controller/hello/hello.go": "package hello\n", "internal/controller/hello/hello_new.go": "package hello\n", "internal/controller/hello/hello_v1_hello.go": "package hello\n", "internal/logic/hello/hello.go": "package hello\n", "internal/model/testmodel.go": "package model\n", "internal/service/hello.go": "package service\n",
	}
	skills := []string{
		"gonex-create-resource", "gonex-design-api", "gonex-implement-controller", "gonex-implement-service",
		"gonex-review-project", "gonex-use-config", "gonex-use-dao", "gonex-use-data", "gonex-use-logging",
		"gonex-use-template",
	}
	for _, skill := range skills {
		directory := ".agents/skills/" + skill
		files[directory+"/SKILL.md"] = "---\nname: " + skill + "\n---\n"
		files[directory+"/agents/openai.yaml"] = skillMetadata(skill, true)
		files["AGENTS.md"] += "$" + skill + "\n"
	}
	for name, content := range map[string]string{
		"gonex-create-resource/references/workflow.md":                 "# Workflow\n",
		"gonex-design-api/references/parameters.md":                    "# Parameters\n",
		"gonex-implement-controller/references/controller-patterns.md": "# Controller patterns\n",
		"gonex-implement-service/references/service-patterns.md":       "# Service patterns\n",
		"gonex-review-project/references/conventions.md":               "# Conventions\n",
	} {
		files[".agents/skills/"+name] = content
	}
	if mutate != nil {
		mutate(files)
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

func skillMetadata(name string, implicit bool) string {
	return "interface:\n  default_prompt: \"Use $" + name + " for this task.\"\npolicy:\n  allow_implicit_invocation: " + fmt.Sprint(implicit) + "\n"
}
