package dao

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

func formatGeneratedModelOutput(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			formatted, formatErr := format.Source(content)
			if formatErr != nil {
				return fmt.Errorf("format generated model %s: %w", path, formatErr)
			}
			if bytes.Equal(content, formatted) {
				return nil
			}
			return os.WriteFile(path, formatted, info.Mode().Perm())
		})
		if err != nil {
			return fmt.Errorf("format generated model output %s: %w", root, err)
		}
	}
	return nil
}

func validateGeneratedModelOutput(roots ...string) error {
	for _, root := range roots {
		files := 0
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			files++
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			if err := validateFileStructTags(file); err != nil {
				return fmt.Errorf("validate generated model %s: %w", path, err)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("validate generated model output %s: %w", root, err)
		}
		if files == 0 {
			return fmt.Errorf("generated model output %s contains no Go files", root)
		}
	}
	return nil
}

func sanitizeGeneratedStructTags(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			changed := false
			var tagErr error
			ast.Inspect(file, func(node ast.Node) bool {
				field, ok := node.(*ast.Field)
				if !ok || field.Tag == nil || tagErr != nil {
					return tagErr == nil
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					tagErr = err
					return false
				}
				updated, sanitized, err := sanitizeGORMStructTag(tag)
				if err != nil {
					tagErr = err
					return false
				}
				if sanitized {
					field.Tag.Value = "`" + updated + "`"
					changed = true
				}
				return true
			})
			if tagErr != nil {
				return fmt.Errorf("sanitize generated struct tag in %s: %w", path, tagErr)
			}
			if !changed {
				return nil
			}
			var formatted bytes.Buffer
			if err := format.Node(&formatted, fileSet, file); err != nil {
				return fmt.Errorf("format generated model %s: %w", path, err)
			}
			formatted.WriteByte('\n')
			return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
		})
		if err != nil {
			return fmt.Errorf("sanitize generated struct tags in %s: %w", root, err)
		}
	}
	return nil
}

func sanitizeGORMStructTag(tag string) (string, bool, error) {
	const prefix = `gorm:"`
	start := strings.Index(tag, prefix)
	if start < 0 || (start > 0 && tag[start-1] != ' ') {
		return tag, false, nil
	}
	valueStart := start + len(prefix)
	valueEnd := -1
	for index := valueStart; index < len(tag); index++ {
		if tag[index] != '"' || escapedAt(tag, index) {
			continue
		}
		if validStructTagBoundary(tag[index+1:]) {
			valueEnd = index
			break
		}
	}
	if valueEnd < 0 {
		return "", false, fmt.Errorf("gorm struct tag has no closing quote: %q", tag)
	}
	value := tag[valueStart:valueEnd]
	var builder strings.Builder
	changed := false
	for index := 0; index < len(value); index++ {
		if value[index] == '"' && !escapedAt(value, index) {
			builder.WriteByte('\\')
			changed = true
		}
		builder.WriteByte(value[index])
	}
	if !changed {
		return tag, false, nil
	}
	return tag[:valueStart] + builder.String() + tag[valueEnd:], true, nil
}

func escapedAt(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func validStructTagBoundary(remainder string) bool {
	if remainder == "" {
		return true
	}
	if remainder[0] != ' ' {
		return false
	}
	remainder = strings.TrimLeft(remainder, " ")
	if remainder == "" {
		return true
	}
	return validateStructTag(remainder) == nil
}

func validateFileStructTags(file *ast.File) error {
	var validationErr error
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || field.Tag == nil || validationErr != nil {
			return validationErr == nil
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			validationErr = err
			return false
		}
		validationErr = validateStructTag(tag)
		return validationErr == nil
	})
	return validationErr
}

func validateStructTag(tag string) error {
	structTag := reflect.StructTag(tag)
	remaining := tag
	for remaining != "" {
		remaining = strings.TrimLeft(remaining, " ")
		if remaining == "" {
			return nil
		}
		colon := strings.IndexByte(remaining, ':')
		if colon <= 0 || colon+1 >= len(remaining) || remaining[colon+1] != '"' {
			return fmt.Errorf("invalid struct tag %q", tag)
		}
		key := remaining[:colon]
		for _, character := range key {
			if character <= ' ' || character == '"' || character == ':' || character == 0x7f {
				return fmt.Errorf("invalid struct tag key %q", key)
			}
		}
		quoted := remaining[colon+1:]
		end := 1
		for end < len(quoted) {
			if quoted[end] == '"' && !escapedAt(quoted, end) {
				break
			}
			end++
		}
		if end >= len(quoted) {
			return fmt.Errorf("unterminated struct tag value for %q", key)
		}
		if _, err := strconv.Unquote(quoted[:end+1]); err != nil {
			return fmt.Errorf("invalid struct tag value for %q: %w", key, err)
		}
		if _, ok := structTag.Lookup(key); !ok {
			return fmt.Errorf("reflect rejected struct tag key %q in %q", key, tag)
		}
		remaining = quoted[end+1:]
		if remaining != "" && remaining[0] != ' ' {
			return fmt.Errorf("struct tag values are not space-separated: %q", tag)
		}
	}
	return nil
}

// removeGeneratedImportAliases keeps generated code idiomatic and consistent
// with the other gx generators. GORM Gen may emit an explicit package alias
// when a schema directory name differs from the package name (for example,
// `entity ".../entity/public"`); the package declaration already supplies
// the correct identifier, so the alias is unnecessary.
func removeGeneratedImportAliases(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			changed := false
			for _, imported := range file.Imports {
				if imported.Name != nil && imported.Name.Name != "_" && imported.Name.Name != "." {
					imported.Name = nil
					changed = true
				}
			}
			if !changed {
				return nil
			}
			var formatted bytes.Buffer
			if err := format.Node(&formatted, fileSet, file); err != nil {
				return fmt.Errorf("format generated model %s: %w", path, err)
			}
			formatted.WriteByte('\n')
			return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove generated model import aliases in %s: %w", root, err)
		}
	}
	return nil
}

func moveGeneratedFiles(sourceRoot, destinationRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(destinationRoot, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, destination); err != nil {
			return err
		}
		return nil
	})
}

func projectImportPath(project Project, absolutePath string) string {
	relative, err := filepath.Rel(project.Root, absolutePath)
	if err != nil {
		return ""
	}
	return strings.TrimRight(project.ModulePath, "/") + "/" + filepath.ToSlash(relative)
}

func rewriteGeneratedImport(root, oldImport, newImport string) error {
	if oldImport == "" || newImport == "" || oldImport == newImport {
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse generated import file %s: %w", path, err)
		}
		changed := false
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote generated import in %s: %w", path, err)
			}
			if importPath != oldImport && !strings.HasPrefix(importPath, oldImport+"/") {
				continue
			}
			imported.Path.Value = strconv.Quote(newImport + strings.TrimPrefix(importPath, oldImport))
			changed = true
		}
		if !changed {
			return nil
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, file); err != nil {
			return fmt.Errorf("format generated import file %s: %w", path, err)
		}
		formatted.WriteByte('\n')
		return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
	})
}
