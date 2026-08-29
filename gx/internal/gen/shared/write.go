package shared

import (
	"bytes"
	"fmt"
	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// OutputMode describes gx's ownership rule for one rendered file.
type OutputMode uint8

const (
	OutputPlanned OutputMode = iota
	OutputForced
	OutputReplacing
	OutputDeveloperOwned
	OutputUpdated
	OutputDelete
)

// Output is a rendered, project-relative file candidate. Content is formatted
// and ownership-checked as one batch before any candidate is published.
type Output struct {
	Path    string
	Content []byte
	Mode    OutputMode
	Label   string
}

// PreparedOutput is an ownership-approved write or deletion ready for a
// single fs.Transaction.
type PreparedOutput struct {
	Path    string
	Content []byte
	Delete  bool
}

// sameFormattedSource compares Go source after formatting the existing file.
// This makes dry-run and ownership decisions independent of checkout line
// endings (notably CRLF on Windows) and harmless formatting differences.
func sameFormattedSource(existing, formatted []byte) bool {
	if bytes.Equal(existing, formatted) {
		return true
	}
	canonical, err := format.Source(existing)
	return err == nil && bytes.Equal(canonical, formatted)
}

// PrepareOutputs formats every candidate and computes its complete result
// before a transaction is opened. This is the common Render→Format boundary
// for controller and service generation.
func PrepareOutputs(project Project, outputs []Output, dryRun bool) (Result, []PreparedOutput, error) {
	var result Result
	prepared := make([]PreparedOutput, 0, len(outputs))
	for _, output := range outputs {
		path := filepath.Clean(output.Path)
		relative, err := filepath.Rel(project.Root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return result, nil, fmt.Errorf("output path must be inside project root: %s", output.Path)
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil && !os.IsNotExist(readErr) {
			return result, nil, fmt.Errorf("read %s: %w", relative, readErr)
		}
		if output.Mode == OutputDelete {
			if readErr == nil && generatedFile(existing) {
				result.Add("DELETE", relative, "gx-owned stale contract")
				prepared = append(prepared, PreparedOutput{Path: relative, Delete: true})
			}
			continue
		}
		content := output.Content
		if output.Mode == OutputDeveloperOwned && readErr == nil && bytes.HasPrefix(existing, []byte(generatedHeader)) {
			content = bytes.TrimLeft(bytes.TrimPrefix(existing, []byte(generatedHeader)), "\r\n")
		}
		formatted, err := format.Source(content)
		if err != nil {
			return result, nil, fmt.Errorf("format %s: %w", relative, err)
		}
		// Forced/replacing output is still a no-op when the complete formatted
		// artifact already matches. This keeps dry-run plans deterministic and
		// avoids reporting needless rewrites for canonical generated projects.
		if readErr == nil && sameFormattedSource(existing, formatted) {
			result.Add("SKIP", relative, "unchanged")
			continue
		}
		switch output.Mode {
		case OutputPlanned:
			if readErr == nil && sameFormattedSource(existing, formatted) {
				result.Add("SKIP", relative, "unchanged")
				continue
			}
			if readErr == nil && !generatedFile(existing) {
				result.Add("WARNING", relative, "existing file is not owned by gx; skipped")
				continue
			}
		case OutputDeveloperOwned:
			if readErr == nil && !bytes.HasPrefix(existing, []byte(generatedHeader)) {
				result.Add("SKIP", relative, "developer-owned "+output.Label+" exists")
				continue
			}
			if readErr == nil {
				result.Add("UPDATE", relative, "generated header removed; developer-owned content preserved")
			} else {
				result.Add("CREATE", relative, "developer-owned after creation")
			}
			prepared = append(prepared, PreparedOutput{Path: relative, Content: formatted})
			continue
		case OutputUpdated:
			if readErr == nil && sameFormattedSource(existing, formatted) {
				result.Add("SKIP", relative, "unchanged")
				continue
			}
		case OutputForced, OutputReplacing:
		default:
			return result, nil, fmt.Errorf("unknown output mode for %s", relative)
		}
		kind := "CREATE"
		detail := ""
		if readErr == nil {
			kind = "UPDATE"
		}
		if output.Mode == OutputForced {
			detail = "generated output replaced"
		}
		if output.Mode == OutputReplacing {
			kind, detail = "UPDATE", "service output replaced"
		}
		if dryRun {
			detail = "dry-run; " + detail
		}
		result.Add(kind, relative, detail)
		prepared = append(prepared, PreparedOutput{Path: relative, Content: formatted})
	}
	return result, prepared, nil
}

// StageOutputs puts all prepared writes in one transaction. A dry run does
// not create a transaction or touch the target project.
func StageOutputs(project Project, outputs []PreparedOutput, dryRun bool) (*genfs.Transaction, error) {
	if dryRun || len(outputs) == 0 {
		return nil, nil
	}
	transaction, err := genfs.NewTransaction(project.Root)
	if err != nil {
		return nil, err
	}
	for _, output := range outputs {
		if output.Delete {
			err = transaction.Delete(output.Path)
		} else {
			err = transaction.Write(output.Path, output.Content, 0o644)
		}
		if err != nil {
			_ = transaction.Rollback()
			return nil, err
		}
	}
	return transaction, nil
}

func writePlanned(project Project, result *Result, path string, source []byte, dryRun bool) error {
	path = filepath.Clean(path)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	content, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format %s: %w", relative, err)
	}
	existing, readErr := os.ReadFile(path)
	switch {
	case readErr == nil && sameFormattedSource(existing, content):
		result.Add("SKIP", relative, "unchanged")
		return nil
	case readErr == nil && !generatedFile(existing):
		result.Add("WARNING", relative, "existing file is not owned by gx; skipped")
		return nil
	case readErr != nil && !os.IsNotExist(readErr):
		return fmt.Errorf("read %s: %w", relative, readErr)
	}
	if dryRun {
		kind := "CREATE"
		if readErr == nil {
			kind = "UPDATE"
		}
		result.Add(kind, relative, "dry-run")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", relative, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gx-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", relative, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions for %s: %w", relative, err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", relative, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush %s: %w", relative, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", relative, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace %s: %w", relative, err)
	}
	kind := "CREATE"
	if readErr == nil {
		kind = "UPDATE"
	}
	result.Add(kind, relative, "")
	return nil
}

// WritePlanned formats and transactionally publishes a generator-owned file.
func WritePlanned(project Project, result *Result, path string, source []byte, dryRun bool) error {
	return writePlanned(project, result, path, source, dryRun)
}

// writeForced writes generator-owned output even when an existing file does not
// have the generated marker. Derived API and Service contracts must be replaced
// when their source definitions change.
func writeForced(project Project, result *Result, path string, source []byte, dryRun bool) error {
	path = filepath.Clean(path)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	content, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format %s: %w", relative, err)
	}
	existing, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read %s: %w", relative, readErr)
	}
	if readErr == nil && sameFormattedSource(existing, content) {
		result.Add("SKIP", relative, "unchanged")
		return nil
	}
	kind := "CREATE"
	if readErr == nil {
		kind = "UPDATE"
	}
	if dryRun {
		result.Add(kind, relative, "generated output is always replaced")
		return nil
	}
	if err := commitGeneratedFile(project, relative, content); err != nil {
		return err
	}
	result.Add(kind, relative, "generated output replaced")
	return nil
}

// WriteForced replaces a derived generator file through the file transaction.
func WriteForced(project Project, result *Result, path string, source []byte, dryRun bool) error {
	return writeForced(project, result, path, source, dryRun)
}

// writeReplacing always rewrites generator-owned Service output, including
// when the generated bytes are unchanged. This makes an explicit `gx service`
// invocation an observable refresh instead of reporting a misleading SKIP.
func writeReplacing(project Project, result *Result, path string, source []byte, dryRun bool) error {
	path = filepath.Clean(path)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	content, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format %s: %w", relative, err)
	}
	_, readErr := os.Stat(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("stat %s: %w", relative, readErr)
	}
	if dryRun {
		result.Add("UPDATE", relative, "service output is always replaced")
		return nil
	}
	if err := commitGeneratedFile(project, relative, content); err != nil {
		return err
	}
	result.Add("UPDATE", relative, "service output replaced")
	return nil
}

// WriteReplacing records an explicit refresh of a generator-owned file.
func WriteReplacing(project Project, result *Result, path string, source []byte, dryRun bool) error {
	return writeReplacing(project, result, path, source, dryRun)
}

// writeDeveloperOwned creates a seed file once and never replaces developer
// content. Older gx versions marked these files as generated; when that exact
// header is present, gx removes only the header and preserves the file body so
// ownership can be transferred without losing work.
func writeDeveloperOwned(project Project, result *Result, path string, source []byte, label string, dryRun bool) error {
	path = filepath.Clean(path)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	existing, readErr := os.ReadFile(path)
	transferringOwnership := readErr == nil && bytes.HasPrefix(existing, []byte(generatedHeader))
	switch {
	case readErr == nil && !transferringOwnership:
		result.Add("SKIP", relative, "developer-owned "+label+" exists")
		return nil
	case readErr != nil && !os.IsNotExist(readErr):
		return fmt.Errorf("read %s: %w", relative, readErr)
	}

	content := source
	kind := "CREATE"
	detail := "developer-owned after creation"
	if transferringOwnership {
		content = bytes.TrimPrefix(existing, []byte(generatedHeader))
		content = bytes.TrimLeft(content, "\r\n")
		kind = "UPDATE"
		detail = "generated header removed; developer-owned content preserved"
	}
	content, err = format.Source(content)
	if err != nil {
		return fmt.Errorf("format %s: %w", relative, err)
	}
	if dryRun {
		result.Add(kind, relative, "dry-run; "+detail)
		return nil
	}
	if err := commitGeneratedFile(project, relative, content); err != nil {
		return err
	}
	result.Add(kind, relative, detail)
	return nil
}

// WriteDeveloperOwned creates a seed file once without replacing user code.
func WriteDeveloperOwned(project Project, result *Result, path string, source []byte, label string, dryRun bool) error {
	return writeDeveloperOwned(project, result, path, source, label, dryRun)
}

func transferLegacyDeveloperOwnership(project Project, result *Result, path, label string, dryRun bool) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read developer-owned %s %s: %w", label, path, err)
	}
	if !bytes.HasPrefix(existing, []byte(generatedHeader)) {
		return nil
	}
	return writeDeveloperOwned(project, result, path, nil, label, dryRun)
}

// TransferLegacyDeveloperOwnership removes an obsolete generated header.
func TransferLegacyDeveloperOwnership(project Project, result *Result, path, label string, dryRun bool) error {
	return transferLegacyDeveloperOwnership(project, result, path, label, dryRun)
}

// writeUpdated is used for the gx-owned Logic aggregation file. The generator
// changes only blank imports in that file, so it can preserve package comments
// while keeping the generated ownership marker.
func writeUpdated(project Project, result *Result, path string, source []byte, dryRun bool) error {
	path = filepath.Clean(path)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	content, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format %s: %w", relative, err)
	}
	existing, readErr := os.ReadFile(path)
	switch {
	case readErr == nil && sameFormattedSource(existing, content):
		result.Add("SKIP", relative, "unchanged")
		return nil
	case readErr == nil && dryRun:
		result.Add("UPDATE", relative, "dry-run; logic imports synchronized")
		return nil
	case readErr != nil && !os.IsNotExist(readErr):
		return fmt.Errorf("read %s: %w", relative, readErr)
	case readErr != nil && dryRun:
		result.Add("CREATE", relative, "dry-run; logic imports synchronized")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", relative, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gx-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", relative, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions for %s: %w", relative, err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", relative, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush %s: %w", relative, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", relative, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace %s: %w", relative, err)
	}
	kind := "CREATE"
	if readErr == nil {
		kind = "UPDATE"
	}
	result.Add(kind, relative, "logic imports synchronized")
	return nil
}

// WriteUpdated synchronizes a generator-owned aggregate file.
func WriteUpdated(project Project, result *Result, path string, source []byte, dryRun bool) error {
	return writeUpdated(project, result, path, source, dryRun)
}

func generatedFile(source []byte) bool {
	return bytes.Contains(source, []byte(generatedHeader))
}

// IsGeneratedFile reports whether source is owned by gx.
func IsGeneratedFile(source []byte) bool { return generatedFile(source) }

func commitGeneratedFile(project Project, relative string, content []byte) error {
	transaction, err := genfs.NewTransaction(project.Root)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := transaction.Write(relative, content, 0o644); err != nil {
		return err
	}
	return transaction.Commit()
}

func withGeneratedHeader(source []byte) []byte {
	if bytes.HasPrefix(source, []byte(generatedHeader)) {
		return source
	}
	return append([]byte(generatedHeader+"\n\n"), source...)
}

// WithGeneratedHeader adds the gx ownership header when it is absent.
func WithGeneratedHeader(source []byte) []byte { return withGeneratedHeader(source) }

// GeneratedHeader marks a file as owned by gx.
const GeneratedHeader = generatedHeader
