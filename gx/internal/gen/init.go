package gen

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	gomodule "golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	canonicalDemoModule = "github.com/lanechi/gonex/examples/demo"
	canonicalDemoName   = "gonex-demo"
	maxTemplateSize     = 32 << 20
	maxExpandedSize     = 64 << 20
	maxTemplateFileSize = 8 << 20
	maxTemplateEntries  = 2048
)

// InitOptions controls creation of a project from the canonical demo archive.
type InitOptions struct {
	ModulePath  string
	Name        string
	TemplateURL string
	Force       bool
	DryRun      bool
}

// InitProject downloads the matching gonex demo, customizes its module and
// name in staging, then atomically makes it the target directory.
func InitProject(target string, options InitOptions) (result Result, err error) {
	target, err = filepath.Abs(target)
	if err != nil {
		return result, fmt.Errorf("resolve project path: %w", err)
	}
	options.ModulePath = strings.TrimSpace(options.ModulePath)
	if err := gomodule.CheckPath(options.ModulePath); err != nil {
		return result, fmt.Errorf("invalid module path %q: %w", options.ModulePath, err)
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = filepath.Base(target)
	}
	if err := validateProjectName(name); err != nil {
		return result, err
	}
	if err := validateInitTarget(target, options.Force); err != nil {
		return result, err
	}
	url := strings.TrimSpace(options.TemplateURL)
	if url == "" {
		url = templateURL(buildVersion())
	}
	if options.DryRun {
		result.add("CREATE", target, "dry-run; extract demo from "+url)
		return result, nil
	}

	staging, err := os.MkdirTemp(filepath.Dir(target), ".gx-init-*")
	if err != nil {
		return result, fmt.Errorf("create init staging: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := downloadAndExtract(context.Background(), url, staging); err != nil {
		return result, err
	}
	if err := validateDemo(staging); err != nil {
		return result, err
	}
	if err := replaceDemoIdentifiers(staging, options.ModulePath, name); err != nil {
		return result, err
	}
	if err := commitInitTarget(staging, target); err != nil {
		return result, err
	}
	for _, relative := range demoFiles(target) {
		result.add("CREATE", relative, "demo template")
	}
	return result, nil
}

func validateInitTarget(target string, force bool) error {
	volumeRoot := filepath.Clean(filepath.VolumeName(target) + string(os.PathSeparator))
	if filepath.Clean(target) == volumeRoot {
		return fmt.Errorf("init target must not be a filesystem root: %s", target)
	}
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat init target: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("init target is not a directory: %s", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read init target: %w", err)
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("init target %s is not empty; use --force to continue", target)
	}
	return nil
}

func validateProjectName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("project name is required")
	}
	for index, character := range name {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !(letter || digit || index > 0 && (character == '-' || character == '_' || character == '.')) {
			return fmt.Errorf("invalid project name %q: use ASCII letters, digits, dots, underscores, or dashes", name)
		}
	}
	return nil
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path == "github.com/lanechi/gonex/gx" {
		return info.Main.Version
	}
	return "(devel)"
}

func templateURL(version string) string {
	version = strings.TrimSpace(version)
	if semver.IsValid(version) && gomodule.IsPseudoVersion(version) {
		if revision, err := gomodule.PseudoVersionRev(version); err == nil && revision != "" {
			return "https://api.github.com/repos/lanechi/gonex/tarball/" + revision
		}
	}
	if semver.IsValid(version) {
		return "https://github.com/lanechi/gonex/archive/refs/tags/gx/" + version + ".tar.gz"
	}
	return "https://github.com/lanechi/gonex/archive/refs/heads/main.tar.gz"
}

func downloadAndExtract(ctx context.Context, url, destination string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create template request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download template: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download template: unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > maxTemplateSize {
		return fmt.Errorf("download template: response exceeds %d bytes", maxTemplateSize)
	}
	compressed, err := io.ReadAll(io.LimitReader(response.Body, maxTemplateSize+1))
	if err != nil {
		return fmt.Errorf("download template body: %w", err)
	}
	if len(compressed) > maxTemplateSize {
		return fmt.Errorf("download template: response exceeds %d bytes", maxTemplateSize)
	}
	return extractDemo(bytes.NewReader(compressed), destination)
}

func extractDemo(source io.Reader, destination string) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("read template gzip: %w", err)
	}
	defer gzipReader.Close()
	archive := tar.NewReader(gzipReader)
	found := false
	entries := 0
	var expandedSize int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read template archive: %w", err)
		}
		if unsafeArchiveName(header.Name) {
			return fmt.Errorf("unsafe template entry %q", header.Name)
		}
		name, ok := demoArchivePath(header.Name)
		if !ok {
			continue
		}
		found = true
		entries++
		if entries > maxTemplateEntries {
			return fmt.Errorf("template contains more than %d demo entries", maxTemplateEntries)
		}
		// A zero type flag is the original tar regular-file marker and remains
		// valid input even though new archives should emit tar.TypeReg.
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return fmt.Errorf("unsafe template entry %q", header.Name)
		}
		output := filepath.Join(destination, filepath.FromSlash(name))
		if !within(destination, output) {
			return fmt.Errorf("template path escapes destination: %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(output, 0o755); err != nil {
				return fmt.Errorf("create template directory: %w", err)
			}
			continue
		}
		if header.Size < 0 || header.Size > maxTemplateFileSize || expandedSize > maxExpandedSize-header.Size {
			return fmt.Errorf("template file %q exceeds extraction limits", header.Name)
		}
		expandedSize += header.Size
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return fmt.Errorf("create template parent: %w", err)
		}
		file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create template file: %w", err)
		}
		_, copyErr := io.CopyN(file, archive, header.Size)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("extract template file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close template file: %w", closeErr)
		}
	}
	if !found {
		return fmt.Errorf("template archive does not contain examples/demo/")
	}
	return nil
}

func demoArchivePath(name string) (string, bool) {
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "examples" || parts[2] != "demo" {
		return "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return strings.Join(parts[3:], "/"), true
}

func unsafeArchiveName(name string) bool {
	if strings.Contains(name, "\\") || path.IsAbs(name) {
		return true
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validateDemo(root string) error {
	for _, relative := range []string{
		"go.mod", "main.go", "README.md", "AGENTS.md", ".env.example", ".gitignore", ".codex/config.toml",
		".codex/agents/architect.toml", ".codex/agents/worker.toml", ".codex/agents/reviewer.toml",
		".codex/agents/explorer.toml", ".codex/agents/tester.toml",
		".agents/skills/gonex-create-resource/SKILL.md", ".agents/skills/gonex-design-api/SKILL.md",
		".agents/skills/gonex-implement-controller/SKILL.md", ".agents/skills/gonex-implement-service/SKILL.md",
		".agents/skills/gonex-review-project/SKILL.md", "api/hello/v1/hello.go",
		"internal/database/database.go", "internal/cmd/cmd.go", "internal/cmd/root.go",
		"internal/controller/hello/hello_v1_hello.go", "internal/logic/hello/hello.go", "internal/service/hello.go",
	} {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || info.IsDir() {
			return fmt.Errorf("template missing required file %s", relative)
		}
	}
	moduleSource, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fmt.Errorf("read template go.mod: %w", err)
	}
	moduleFile, err := modfile.Parse("go.mod", moduleSource, nil)
	if err != nil || moduleFile.Module == nil || moduleFile.Module.Mod.Path != canonicalDemoModule {
		return fmt.Errorf("template go.mod must declare module %s", canonicalDemoModule)
	}
	rootSource, err := os.ReadFile(filepath.Join(root, "internal/cmd/root.go"))
	if err != nil {
		return fmt.Errorf("read template project name: %w", err)
	}
	if !bytes.Contains(rootSource, []byte(canonicalDemoName)) {
		return fmt.Errorf("template project name marker %q is missing", canonicalDemoName)
	}
	gitignoreSource, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return fmt.Errorf("read template .gitignore: %w", err)
	}
	if !ignoresRootEnv(string(gitignoreSource)) {
		return fmt.Errorf("template .gitignore must ignore the root .env file")
	}
	databaseSource, err := os.ReadFile(filepath.Join(root, "internal/database/database.go"))
	if err != nil {
		return fmt.Errorf("read template database: %w", err)
	}
	if !bytes.Contains(databaseSource, []byte("gorm.io/driver/postgres")) {
		return fmt.Errorf("template database must use PostgreSQL")
	}
	for _, unsupported := range []string{"gorm.io/driver/mysql", "gorm.io/driver/sqlite", "gorm.io/driver/sqlserver"} {
		if bytes.Contains(databaseSource, []byte(unsupported)) {
			return fmt.Errorf("template database contains unsupported driver %s", unsupported)
		}
	}
	return nil
}

func ignoresRootEnv(source string) bool {
	ignored := false
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == ".env" || line == "/.env" {
			ignored = true
		}
		if line == "!.env" || line == "!/.env" {
			ignored = false
		}
	}
	return ignored
}

func replaceDemoIdentifiers(root, modulePath, name string) error {
	if err := filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(content), canonicalDemoModule, modulePath)
		updated = strings.ReplaceAll(updated, canonicalDemoName, name)
		if updated != string(content) {
			return os.WriteFile(file, []byte(updated), 0o644)
		}
		return nil
	}); err != nil {
		return err
	}
	return removeRepositoryLocalReplace(filepath.Join(root, "go.mod"))
}

func removeRepositoryLocalReplace(goModPath string) error {
	source, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("read generated go.mod: %w", err)
	}
	file, err := modfile.Parse(goModPath, source, nil)
	if err != nil {
		return fmt.Errorf("parse generated go.mod: %w", err)
	}
	if err := file.DropRequire("github.com/lanechi/gonex"); err != nil {
		return fmt.Errorf("remove template framework requirement: %w", err)
	}
	if err := file.DropReplace("github.com/lanechi/gonex", ""); err != nil {
		return fmt.Errorf("remove template framework replacement: %w", err)
	}
	formatted, err := file.Format()
	if err != nil {
		return fmt.Errorf("format generated go.mod: %w", err)
	}
	if err := os.WriteFile(goModPath, formatted, 0o644); err != nil {
		return fmt.Errorf("write generated go.mod: %w", err)
	}
	return nil
}

func commitInitTarget(staging, target string) error {
	return commitInitTargetWithRemove(staging, target, os.RemoveAll)
}

func commitInitTargetWithRemove(staging, target string, removeAll func(string) error) error {
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create init parent: %w", err)
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := os.Rename(staging, target); err != nil {
			return fmt.Errorf("commit init project: %w", err)
		}
		return nil
	}
	backup, err := os.MkdirTemp(filepath.Dir(target), ".gx-backup-*")
	if err != nil {
		return fmt.Errorf("create init backup: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare init backup: %w", err)
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("backup existing project: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return fmt.Errorf("commit init project: %w; rollback failed: %v; backup retained at %s", err, rollbackErr, backup)
		}
		return fmt.Errorf("commit init project: %w", err)
	}
	if err := removeAll(backup); err != nil {
		return fmt.Errorf("project initialized but remove backup %s: %w; backup retained", backup, err)
	}
	return nil
}

func demoFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if relative, relativeErr := filepath.Rel(root, file); relativeErr == nil {
				files = append(files, filepath.ToSlash(relative))
			}
		}
		return nil
	})
	sort.Strings(files)
	return files
}
