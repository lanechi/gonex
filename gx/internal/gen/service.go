package gen

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func GenerateServices(project Project, options ServiceOptions) (Result, error) {
	if strings.TrimSpace(options.Name) != "" {
		return generateStandardService(project, options)
	}
	var result Result
	if options.Source == "" {
		options.Source = defaultLogicSource
	}
	if options.Destination == "" {
		options.Destination = defaultServiceDest
	}
	modules, err := scanLogic(project, options.Source, options.Module)
	if err != nil {
		return result, err
	}
	logicSources := make(map[string]struct{})
	for _, methods := range modules {
		for _, method := range methods {
			if _, exists := logicSources[method.Source]; exists {
				continue
			}
			logicSources[method.Source] = struct{}{}
			if err := transferLegacyDeveloperOwnership(project, &result, project.Resolve(method.Source), "Logic implementation", options.DryRun); err != nil {
				return result, err
			}
		}
	}
	if err := ensureDemoModel(project, &result, options.DryRun); err != nil {
		return result, err
	}
	moduleNames := make([]string, 0, len(modules))
	for module := range modules {
		moduleNames = append(moduleNames, module)
	}
	sort.Strings(moduleNames)
	for _, module := range moduleNames {
		methods := modules[module]
		source, err := renderService(project, options.Destination, module, methods)
		if err != nil {
			return result, err
		}
		path := filepath.Join(project.Resolve(options.Destination), module+".go")
		if err := writeReplacing(project, &result, path, source, options.DryRun); err != nil {
			return result, err
		}
	}
	if err := syncLogicAggregator(project, options.Source, modules, &result, options.DryRun); err != nil {
		return result, err
	}
	return result, nil
}

func scanLogic(project Project, sourceDir, requestedModule string) (map[string][]LogicMethod, error) {
	root := project.Resolve(sourceDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("logic source directory %s does not exist", sourceDir)
		}
		return nil, fmt.Errorf("read logic source directory: %w", err)
	}
	modules := make(map[string][]LogicMethod)
	for _, entry := range entries {
		if !entry.IsDir() || !validIdentifier(entry.Name()) || (requestedModule != "" && entry.Name() != requestedModule) {
			continue
		}
		moduleDir := filepath.Join(root, entry.Name())
		files, err := os.ReadDir(moduleDir)
		if err != nil {
			return nil, fmt.Errorf("read logic module %s: %w", entry.Name(), err)
		}
		for _, fileEntry := range files {
			if fileEntry.IsDir() || filepath.Ext(fileEntry.Name()) != ".go" || strings.HasSuffix(fileEntry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(moduleDir, fileEntry.Name())
			methods, err := parseLogicFile(path)
			if err != nil {
				return nil, err
			}
			for index := range methods {
				methods[index].Source, _ = filepath.Rel(project.Root, path)
			}
			modules[entry.Name()] = append(modules[entry.Name()], methods...)
		}
	}
	if len(modules) == 0 {
		if requestedModule != "" {
			return nil, fmt.Errorf("logic module %s not found under %s", requestedModule, sourceDir)
		}
		return nil, fmt.Errorf("no logic modules found under %s", sourceDir)
	}
	for module, methods := range modules {
		sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
		seen := make(map[string]string)
		for _, method := range methods {
			if previous, exists := seen[method.Name]; exists {
				return nil, fmt.Errorf("duplicate logic method %s in %s and %s", method.Name, previous, method.Source)
			}
			seen[method.Name] = method.Source
		}
		modules[module] = methods
	}
	return modules, nil
}

// syncLogicAggregator keeps the package-level logic import side effect in
// sync with the logic modules used to generate services. Importing each
// implementation package makes its init function register the service.
func syncLogicAggregator(project Project, sourceDir string, modules map[string][]LogicMethod, result *Result, dryRun bool) error {
	importPaths, err := logicImportPaths(project, sourceDir, modules)
	if err != nil {
		return err
	}
	path := filepath.Join(project.Resolve(sourceDir), logicAggregatorName)
	existing, readErr := os.ReadFile(path)
	var source []byte
	switch {
	case readErr == nil:
		source, err = addLogicImports(project, sourceDir, existing, importPaths)
		if err != nil {
			return fmt.Errorf("update logic aggregator %s: %w", path, err)
		}
	case os.IsNotExist(readErr):
		source = renderLogicAggregator(importPaths)
	default:
		return fmt.Errorf("read logic aggregator %s: %w", path, readErr)
	}
	source = withGeneratedHeader(source)
	return writeUpdated(project, result, path, source, dryRun)
}

func logicImportPaths(project Project, sourceDir string, modules map[string][]LogicMethod) ([]string, error) {
	sourceRoot := project.Resolve(sourceDir)
	relative, err := filepath.Rel(project.Root, sourceRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("logic source directory must be inside the project root: %s", sourceDir)
	}
	base := filepath.ToSlash(relative)
	if base == "." {
		base = ""
	}
	modulePrefix := strings.TrimRight(project.ModulePath, "/")
	paths := make([]string, 0, len(modules))
	for module := range modules {
		importPath := modulePrefix
		if base != "" {
			importPath += "/" + strings.Trim(base, "/")
		}
		importPath += "/" + module
		paths = append(paths, importPath)
	}
	sort.Strings(paths)
	return paths, nil
}

func addLogicImports(project Project, sourceDir string, source []byte, importPaths []string) ([]byte, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, logicAggregatorName, source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if file.Name == nil || file.Name.Name != "logic" {
		return nil, fmt.Errorf("package must be logic")
	}
	desired := make(map[string]struct{}, len(importPaths))
	for _, importPath := range importPaths {
		desired[importPath] = struct{}{}
	}
	for _, declaration := range file.Decls {
		importDecl, ok := declaration.(*ast.GenDecl)
		if !ok || importDecl.Tok != token.IMPORT {
			continue
		}
		specs := importDecl.Specs[:0]
		for _, spec := range importDecl.Specs {
			imported, ok := spec.(*ast.ImportSpec)
			if !ok {
				specs = append(specs, spec)
				continue
			}
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return nil, err
			}
			if imported.Name != nil && imported.Name.Name == "_" && managedLogicImport(project, sourceDir, path) {
				if _, keep := desired[path]; !keep {
					continue
				}
			}
			specs = append(specs, spec)
		}
		importDecl.Specs = specs
	}
	existing := make(map[string]struct{}, len(file.Imports))
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		existing[path] = struct{}{}
	}
	missing := make([]string, 0, len(importPaths))
	for _, importPath := range importPaths {
		if _, ok := existing[importPath]; !ok {
			missing = append(missing, importPath)
		}
	}
	if len(missing) == 0 {
		var builder strings.Builder
		if err := format.Node(&builder, fileSet, file); err != nil {
			return nil, err
		}
		builder.WriteByte('\n')
		return []byte(builder.String()), nil
	}

	var importDecl *ast.GenDecl
	for _, declaration := range file.Decls {
		candidate, ok := declaration.(*ast.GenDecl)
		if ok && candidate.Tok == token.IMPORT {
			importDecl = candidate
			break
		}
	}
	if importDecl == nil {
		importDecl = &ast.GenDecl{Tok: token.IMPORT}
		file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
	}
	for _, importPath := range missing {
		importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
			Name: ast.NewIdent("_"),
			Path: &ast.BasicLit{Kind: token.STRING, Value: strconvQuote(importPath)},
		})
	}
	var builder strings.Builder
	if err := format.Node(&builder, fileSet, file); err != nil {
		return nil, err
	}
	builder.WriteByte('\n')
	return []byte(builder.String()), nil
}

func managedLogicImport(project Project, sourceDir, importPath string) bool {
	relative, err := filepath.Rel(project.Root, project.Resolve(sourceDir))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	prefix := strings.TrimRight(project.ModulePath, "/")
	if relative != "." {
		prefix += "/" + strings.Trim(filepath.ToSlash(relative), "/")
	}
	prefix += "/"
	module := strings.TrimPrefix(importPath, prefix)
	return module != importPath && validIdentifier(module)
}

func renderLogicAggregator(importPaths []string) []byte {
	var builder strings.Builder
	builder.WriteString("// Package logic imports all business logic implementations so their init\n")
	builder.WriteString("// functions can register implementations with the generated service package.\n")
	builder.WriteString("package logic\n\n")
	builder.WriteString("import (\n")
	for _, importPath := range importPaths {
		fmt.Fprintf(&builder, "\t_ %q\n", importPath)
	}
	builder.WriteString(")\n")
	return []byte(builder.String())
}

func parseLogicFile(path string) ([]LogicMethod, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse logic file %s: %w", path, err)
	}
	imports, err := importRefs(file)
	if err != nil {
		return nil, fmt.Errorf("parse imports in %s: %w", path, err)
	}
	var methods []LogicMethod
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || function.Name == nil || !ast.IsExported(function.Name.Name) {
			continue
		}
		if len(function.Recv.List) != 1 {
			return nil, fmt.Errorf("logic method %s in %s has an unsupported receiver", function.Name.Name, path)
		}
		usedImports := usedSignatureImports(function.Type, imports)
		methods = append(methods, LogicMethod{
			Name: function.Name.Name, Doc: commentText(function.Doc),
			Signature: functionSignature(function.Type, fileSet), Imports: usedImports,
		})
	}
	return methods, nil
}

func renderService(project Project, destination, module string, methods []LogicMethod) ([]byte, error) {
	imports := make(map[string]ImportRef)
	serviceImportPath := projectImportPathForDirectory(project, destination)
	for _, method := range methods {
		for _, imported := range method.Imports {
			if imported.Path == serviceImportPath {
				continue
			}
			imports[imported.Path] = imported
		}
	}
	var builder strings.Builder
	builder.WriteString(generatedHeader)
	builder.WriteString("\n\npackage service\n\n")
	if len(imports) > 0 {
		paths := make([]string, 0, len(imports))
		for path := range imports {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		builder.WriteString("import (\n")
		for _, path := range paths {
			fmt.Fprintf(&builder, "\t%q\n", imports[path].Path)
		}
		builder.WriteString(")\n\n")
	}
	builder.WriteString("type I")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" interface {\n")
	for _, method := range methods {
		if method.Doc != "" {
			for _, line := range strings.Split(method.Doc, "\n") {
				builder.WriteString("\t// ")
				builder.WriteString(strings.TrimSpace(line))
				builder.WriteByte('\n')
			}
		}
		builder.WriteString("\t")
		builder.WriteString(method.Name)
		signature := strings.TrimPrefix(method.Signature, "func")
		for _, imported := range method.Imports {
			if imported.Path == serviceImportPath {
				signature = strings.ReplaceAll(signature, imported.Name+".", "")
			}
		}
		builder.WriteString(signature)
		builder.WriteByte('\n')
	}
	builder.WriteString("}\n")
	builder.WriteString("\nvar local")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" I")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString("\n\n")
	builder.WriteString("// ")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" returns the registered ")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" service implementation.\n")
	builder.WriteString("func ")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString("() I")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" {\n\tif local")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" == nil {\n\t\tpanic(\"gx: ")
	builder.WriteString(module)
	builder.WriteString(" service is not registered\")\n\t}\n\treturn local")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString("\n}\n\n")
	builder.WriteString("// Register")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" registers the ")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" service implementation.\n")
	builder.WriteString("func Register")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString("(implementation I")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(") {\n\tlocal")
	builder.WriteString(exportedIdentifier(module))
	builder.WriteString(" = implementation\n}\n")
	return []byte(builder.String()), nil
}

func ensureDemoModel(project Project, result *Result, dryRun bool) error {
	path := project.Resolve(defaultDemoModelPath)
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return err
	}
	info, readErr := os.Stat(path)
	switch {
	case readErr == nil && info.IsDir():
		return fmt.Errorf("demo model path is a directory: %s", relative)
	case readErr == nil:
		result.add("SKIP", relative, "demo model exists")
		return nil
	case !os.IsNotExist(readErr):
		return fmt.Errorf("stat demo model %s: %w", relative, readErr)
	}
	return writePlanned(project, result, path, renderDemoModel(), dryRun)
}

func renderDemoModel() []byte {
	return []byte(generatedHeader + `

package model

// TestModel is the placeholder request and response model used by generated
// demo services. Replace it with business-specific models as the project grows.
type TestModel struct {
	ID   int64  ` + "`json:\"id,omitempty\"`" + `
	Name string ` + "`json:\"name,omitempty\"`" + `
}

// TestModelList is the non-paginated list response model for demo services.
type TestModelList struct {
	Items []*TestModel ` + "`json:\"items\"`" + `
}

// TestModelPage is the paginated list response model for demo services.
type TestModelPage struct {
	Items    []*TestModel ` + "`json:\"items\"`" + `
	Page     int          ` + "`json:\"page\"`" + `
	PageSize int          ` + "`json:\"pageSize\"`" + `
	Total    int64        ` + "`json:\"total\"`" + `
}
`)
}

func demoModelImportPath(project Project) string {
	return strings.TrimRight(project.ModulePath, "/") + "/internal/model"
}

func projectImportPathForDirectory(project Project, directory string) string {
	relative, err := filepath.Rel(project.Root, project.Resolve(directory))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return strings.TrimRight(project.ModulePath, "/") + "/" + filepath.ToSlash(relative)
}

func importRefs(file *ast.File) (map[string]ImportRef, error) {
	result := make(map[string]ImportRef)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
			continue
		}
		if spec.Name != nil {
			return nil, fmt.Errorf("aliased import %q is not supported; use the package's default name", path)
		}
		name := filepath.Base(path)
		result[name] = ImportRef{Name: name, Path: path}
	}
	return result, nil
}

func usedSignatureImports(functionType *ast.FuncType, imports map[string]ImportRef) []ImportRef {
	used := make(map[string]ImportRef)
	ast.Inspect(functionType, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if imported, exists := imports[identifier.Name]; exists {
			used[identifier.Name] = imported
		}
		return true
	})
	result := make([]ImportRef, 0, len(used))
	for _, imported := range used {
		result = append(result, imported)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func functionSignature(functionType *ast.FuncType, fileSet *token.FileSet) string {
	var builder strings.Builder
	_ = format.Node(&builder, fileSet, functionType)
	return builder.String()
}

func commentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Text())
}
