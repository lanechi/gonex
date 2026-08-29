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
// before a transaction is opened. This is the one Render→Format ownership
// boundary used by controller and service generation.
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
		if readErr == nil && sameFormattedSource(existing, formatted) {
			result.Add("SKIP", relative, "unchanged")
			continue
		}

		switch output.Mode {
		case OutputPlanned:
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
		case OutputUpdated, OutputForced, OutputReplacing:
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
		if output.Mode == OutputUpdated {
			detail = "logic imports synchronized"
		}
		if dryRun {
			if detail == "" {
				detail = "dry-run"
			} else {
				detail = "dry-run; " + detail
			}
		}
		result.Add(kind, relative, detail)
		prepared = append(prepared, PreparedOutput{Path: relative, Content: formatted})
	}
	return result, prepared, nil
}

// StageOutputs puts every prepared write/delete in one filesystem transaction.
// There is intentionally no second direct-write publication API: generator
// pipelines must publish through this transaction boundary.
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

func generatedFile(source []byte) bool {
	return bytes.Contains(source, []byte(generatedHeader))
}

// IsGeneratedFile reports whether source is owned by gx.
func IsGeneratedFile(source []byte) bool { return generatedFile(source) }

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
