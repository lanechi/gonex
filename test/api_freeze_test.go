package ghttp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const publicAPIGolden = "testdata/public_api.golden"

// TestAPIFreezeRules preserves compatibility checks for APIs deliberately
// removed before v1. The public API snapshot below protects the remaining
// exported surface from accidental changes.
func TestAPIFreezeRules(t *testing.T) {
	root := filepath.Join("..")
	paths, err := coreProductionGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "NewRedisStorage") || strings.Contains(string(content), "NewOwnedRedisStorage") || strings.Contains(string(content), "type RedisStorage") {
			t.Fatalf("removed API remains in %s", path)
		}
		if strings.Contains(string(content), "addressSet") || strings.Contains(string(content), "openapiSet") || strings.Contains(string(content), "modeSet") {
			t.Fatalf("legacy xxxSet option state remains in %s", path)
		}
	}

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

// TestPublicAPIFreeze compares all exported declarations in the core public
// packages to a deterministic baseline. Set UPDATE_PUBLIC_API_GOLDEN=1 only
// when intentionally reviewing and accepting a public API change.
func TestPublicAPIFreeze(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := publicAPISnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "test", publicAPIGolden)
	if os.Getenv("UPDATE_PUBLIC_API_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read public API baseline: %v; set UPDATE_PUBLIC_API_GOLDEN=1 after reviewing the API change", err)
	}
	if bytes.Equal(expected, actual) {
		return
	}
	t.Fatalf("public API baseline changed:\n%s", describeAPIDiff(expected, actual))
}

type listedPackage struct {
	path string
	dir  string
}

func publicAPISnapshot(root string) ([]byte, error) {
	command := exec.Command("go", "list", "-f", "{{.ImportPath}}\t{{.Dir}}", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list public packages: %w", err)
	}
	var packages []listedPackage
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || !isPublicPackage(root, parts[1]) {
			continue
		}
		packages = append(packages, listedPackage{path: parts[0], dir: parts[1]})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].path < packages[j].path })
	imports, err := exportImporter(root)
	if err != nil {
		return nil, err
	}

	var snapshot strings.Builder
	for _, pkg := range packages {
		declarations, err := exportedTypeDeclarations(pkg.dir, pkg.path, imports)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", pkg.path, err)
		}
		fmt.Fprintf(&snapshot, "PACKAGE %s\n", pkg.path)
		for _, declaration := range declarations {
			fmt.Fprintf(&snapshot, "%s\n", declaration)
		}
	}
	return []byte(snapshot.String()), nil
}

type listedExport struct {
	ImportPath string
	Export     string
}

func exportImporter(root string) (types.Importer, error) {
	command := exec.Command("go", "list", "-deps", "-export", "-json", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list package export data: %w", err)
	}
	exports := make(map[string]string)
	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var item listedExport
		if err := decoder.Decode(&item); err != nil {
			return nil, fmt.Errorf("decode package export data: %w", err)
		}
		if item.ImportPath != "" && item.Export != "" {
			exports[item.ImportPath] = item.Export
		}
	}
	return importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
		filename, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(filename)
	}), nil
}

// exportedTypeDeclarations records the checked Go type surface rather than
// source spelling. In particular it follows aliases, resolves qualified type
// names, includes exported struct fields and complete method sets, and records
// evaluated constant values.
func exportedTypeDeclarations(directory, importPath string, imports types.Importer) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(files)
	fileSet := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(files))
	for _, filename := range files {
		file, parseErr := parser.ParseFile(fileSet, filename, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return nil, parseErr
		}
		parsed = append(parsed, file)
	}
	configuration := types.Config{Importer: imports}
	pkg, err := configuration.Check(importPath, fileSet, parsed, nil)
	if err != nil {
		return nil, err
	}
	return typeDeclarations(pkg), nil
}

func typeDeclarations(pkg *types.Package) []string {
	qualifier := func(imported *types.Package) string { return imported.Path() }
	qualified := func(value types.Type) string { return types.TypeString(value, qualifier) }
	var declarations []string
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if !ast.IsExported(name) {
			continue
		}
		object := scope.Lookup(name)
		switch object := object.(type) {
		case *types.Const:
			declarations = append(declarations, fmt.Sprintf("const %s %s = %s", name, qualified(object.Type()), object.Val().ExactString()))
		case *types.Var:
			declarations = append(declarations, fmt.Sprintf("var %s %s", name, qualified(object.Type())))
		case *types.Func:
			declarations = append(declarations, "func "+name+qualified(object.Type()))
		case *types.TypeName:
			typeName := object.Type()
			if object.IsAlias() {
				declarations = append(declarations, fmt.Sprintf("type %s = %s", name, qualified(typeName)))
			} else {
				underlying := typeName.Underlying()
				if _, ok := underlying.(*types.Struct); ok {
					declarations = append(declarations, fmt.Sprintf("type %s struct{}", name))
				} else {
					declarations = append(declarations, fmt.Sprintf("type %s %s", name, qualified(underlying)))
				}
				if named, ok := typeName.(*types.Named); ok && named.TypeParams() != nil {
					declarations = append(declarations, fmt.Sprintf("typeparams %s %s", name, formatTypeParams(named.TypeParams(), qualified)))
				}
			}
			if structure, ok := object.Type().Underlying().(*types.Struct); ok {
				for index := 0; index < structure.NumFields(); index++ {
					field := structure.Field(index)
					if field.Exported() {
						declarations = append(declarations, fmt.Sprintf("field %s.%s %s %q", name, field.Name(), qualified(field.Type()), structure.Tag(index)))
					}
				}
			}
			for _, receiver := range []types.Type{object.Type(), types.NewPointer(object.Type())} {
				for index := 0; index < types.NewMethodSet(receiver).Len(); index++ {
					method := types.NewMethodSet(receiver).At(index).Obj()
					if method.Exported() {
						declarations = append(declarations, fmt.Sprintf("method %s %s", qualified(receiver), method.Name()+qualified(method.Type())))
					}
				}
			}
		}
	}
	sort.Strings(declarations)
	return compactDeclarations(declarations)
}

func formatTypeParams(params *types.TypeParamList, qualified func(types.Type) string) string {
	var values []string
	for index := 0; index < params.Len(); index++ {
		param := params.At(index)
		values = append(values, param.Obj().Name()+" "+qualified(param.Constraint()))
	}
	return strings.Join(values, ", ")
}

func compactDeclarations(declarations []string) []string {
	if len(declarations) == 0 {
		return nil
	}
	compact := declarations[:0]
	for _, declaration := range declarations {
		if len(compact) == 0 || compact[len(compact)-1] != declaration {
			compact = append(compact, declaration)
		}
	}
	return compact
}

func TestPublicAPIFreezeDetectorTracksSemanticMutations(t *testing.T) {
	makePackage := func(source string) *types.Package {
		t.Helper()
		files, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		fileSet := token.NewFileSet()
		files, err = parser.ParseFile(fileSet, "fixture.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := (&types.Config{}).Check("fixture", fileSet, []*ast.File{files}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	baseline := strings.Join(typeDeclarations(makePackage(`package fixture; const Value = 1; var State string; type Record struct { Field int }; func (Record) Act(string) error { return nil }`)), "\n")
	mutated := strings.Join(typeDeclarations(makePackage(`package fixture; const Value = 2; var State int; type Record struct { Field string }; func (Record) Act(int) error { return nil }`)), "\n")
	if baseline == mutated {
		t.Fatal("semantic API detector missed constant, variable, field, or method mutation")
	}
}

func TestPublicAPIFreezeDetectorTracksAliasesAndGenericConstraints(t *testing.T) {
	makePackage := func(source string) *types.Package {
		t.Helper()
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "fixture.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := (&types.Config{}).Check("fixture", fileSet, []*ast.File{file}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	alias := strings.Join(typeDeclarations(makePackage(`package fixture; type Base struct{}; type Alias = Base`)), "\n")
	defined := strings.Join(typeDeclarations(makePackage(`package fixture; type Base struct{}; type Alias Base`)), "\n")
	if alias == defined {
		t.Fatal("API detector missed alias/defined type mutation")
	}
	anyConstraint := strings.Join(typeDeclarations(makePackage(`package fixture; type Value[T any] struct{}`)), "\n")
	comparableConstraint := strings.Join(typeDeclarations(makePackage(`package fixture; type Value[T comparable] struct{}`)), "\n")
	if anyConstraint == comparableConstraint {
		t.Fatal("API detector missed generic constraint mutation")
	}
}

func isPublicPackage(root, directory string) bool {
	relative, err := filepath.Rel(root, directory)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 0 || parts[0] == "test" {
		return false
	}
	for _, part := range parts {
		if part == "internal" || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

func exportedDeclarations(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(files)
	fileSet := token.NewFileSet()
	declarations := make([]string, 0)
	for _, name := range files {
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, declaration := range file.Decls {
			declarations = append(declarations, exportedDeclarationStrings(fileSet, declaration)...)
		}
	}
	sort.Strings(declarations)
	return declarations, nil
}

func exportedDeclarationStrings(fileSet *token.FileSet, declaration ast.Decl) []string {
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		if declaration.Name.IsExported() && (declaration.Recv == nil || exportedReceiver(declaration.Recv)) {
			return []string{formatFunctionSignature(fileSet, declaration)}
		}
	case *ast.GenDecl:
		var declarations []string
		for _, spec := range declaration.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				if spec.Name.IsExported() {
					declarations = append(declarations, "type "+formatNode(fileSet, spec))
				}
			case *ast.ValueSpec:
				for index, name := range spec.Names {
					if !name.IsExported() {
						continue
					}
					declarations = append(declarations, valueDeclaration(fileSet, declaration.Tok.String(), spec, index))
				}
			}
		}
		return declarations
	}
	return nil
}

func exportedReceiver(receivers *ast.FieldList) bool {
	if receivers == nil || len(receivers.List) != 1 {
		return false
	}
	var receiver ast.Expr = receivers.List[0].Type
	for {
		pointer, ok := receiver.(*ast.StarExpr)
		if !ok {
			break
		}
		receiver = pointer.X
	}
	name, ok := receiver.(*ast.Ident)
	return ok && name.IsExported()
}

func formatFunctionSignature(fileSet *token.FileSet, declaration *ast.FuncDecl) string {
	copy := *declaration
	copy.Doc = nil
	copy.Body = nil
	return formatNode(fileSet, &copy)
}

func valueDeclaration(fileSet *token.FileSet, kind string, spec *ast.ValueSpec, index int) string {
	var declaration strings.Builder
	declaration.WriteString(kind)
	declaration.WriteByte(' ')
	declaration.WriteString(spec.Names[index].Name)
	if spec.Type != nil {
		declaration.WriteByte(' ')
		declaration.WriteString(formatNode(fileSet, spec.Type))
	}
	if index < len(spec.Values) {
		declaration.WriteString(" = ")
		declaration.WriteString(formatNode(fileSet, spec.Values[index]))
	}
	return declaration.String()
}

func formatNode(fileSet *token.FileSet, node any) string {
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, node); err != nil {
		panic(err)
	}
	return strings.TrimSpace(output.String())
}

func describeAPIDiff(expected, actual []byte) string {
	expectedSet := declarationSet(expected)
	actualSet := declarationSet(actual)
	var changes []string
	for declaration := range expectedSet {
		if _, exists := actualSet[declaration]; !exists {
			changes = append(changes, "- removed or signature changed: "+declaration)
		}
	}
	for declaration := range actualSet {
		if _, exists := expectedSet[declaration]; !exists {
			changes = append(changes, "+ added or signature changed: "+declaration)
		}
	}
	sort.Strings(changes)
	return strings.Join(changes, "\n")
}

func declarationSet(snapshot []byte) map[string]struct{} {
	set := make(map[string]struct{})
	packageName := ""
	for _, line := range strings.Split(strings.TrimSpace(string(snapshot)), "\n") {
		if strings.HasPrefix(line, "PACKAGE ") {
			packageName = strings.TrimPrefix(line, "PACKAGE ")
			continue
		}
		if line != "" && packageName != "" {
			set[packageName+"\t"+line] = struct{}{}
		}
	}
	return set
}
