package gen

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func GenerateControllers(project Project, options ControllerOptions) (Result, error) {
	if strings.TrimSpace(options.Name) != "" {
		return generateStandardControllers(project, options)
	}
	var result Result
	if options.Source == "" {
		options.Source = defaultAPISource
	}
	if options.Destination == "" {
		options.Destination = defaultControllerDest
	}
	apis, err := scanAPIs(project, options.Source)
	if err != nil {
		return result, err
	}
	apiSources := make(map[string]struct{})
	for _, api := range apis {
		if _, exists := apiSources[api.Source]; exists {
			continue
		}
		apiSources[api.Source] = struct{}{}
		if err := transferLegacyDeveloperOwnership(project, &result, project.Resolve(api.Source), "API definition", options.DryRun); err != nil {
			return result, err
		}
	}
	groups := make(map[string][]API)
	expectedGenerated := make(map[string]struct{})
	for _, api := range apis {
		key := api.Module + "/" + api.Version
		groups[key] = append(groups[key], api)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		items := groups[key]
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		module, version := items[0].Module, items[0].Version
		packageName := goPackageName(module)
		generatedPath := filepath.Join(project.Resolve(options.Destination), module, module+"_"+version+"_generated.go")
		expectedGenerated[filepath.Clean(generatedPath)] = struct{}{}
		source, err := renderController(project, items, packageName, module, version)
		if err != nil {
			return result, err
		}
		if err := writePlanned(project, &result, generatedPath, source, options.DryRun); err != nil {
			return result, err
		}
		bySource := make(map[string][]API)
		for _, api := range items {
			bySource[api.Source] = append(bySource[api.Source], api)
		}
		sources := make([]string, 0, len(bySource))
		for sourceName := range bySource {
			sources = append(sources, sourceName)
		}
		sort.Strings(sources)
		for _, sourceName := range sources {
			fileStem := strings.TrimSuffix(filepath.Base(sourceName), filepath.Ext(sourceName))
			implementationPath := filepath.Join(project.Resolve(options.Destination), module, module+"_"+version+"_"+toSnake(fileStem)+".go")
			sourceAPIs := bySource[sourceName]
			implementation, err := renderControllerMethods(sourceAPIs, packageName)
			if err != nil {
				return result, err
			}
			if err := writeDeveloperOwned(project, &result, implementationPath, implementation, "controller implementation", options.DryRun); err != nil {
				return result, err
			}
		}
	}
	if options.Clean {
		if err := cleanStaleGeneratedContracts(project, options.Destination, expectedGenerated, &result, options.DryRun); err != nil {
			return result, err
		}
	}
	return result, nil
}

func renderController(project Project, apis []API, packageName, module, version string) ([]byte, error) {
	imports := map[string]string{"context": "context"}
	for _, api := range apis {
		imports[api.ImportPath] = api.Package
	}
	if err := validateImportNames(imports); err != nil {
		return nil, err
	}
	file := &ast.File{Name: ast.NewIdent(packageName)}
	file.Comments = nil
	file.Decls = append(file.Decls, importDeclaration(imports))
	interfaceFields := make([]*ast.Field, 0, len(apis))
	for _, api := range apis {
		interfaceFields = append(interfaceFields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(api.Name)},
			Type: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("ctx")}, Type: selector("context", "Context")},
					{Names: []*ast.Ident{ast.NewIdent("req")}, Type: &ast.StarExpr{X: selector(api.Package, api.RequestType)}},
				}},
				Results: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.StarExpr{X: selector(api.Package, api.ResponseType)}},
					{Type: ast.NewIdent("error")},
				}},
			},
		})
	}
	interfaceName := "I" + exportedIdentifier(module) + exportedIdentifier(version)
	controllerName := "Controller" + exportedIdentifier(version)
	file.Decls = append(file.Decls,
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(interfaceName), Type: &ast.InterfaceType{Methods: &ast.FieldList{List: interfaceFields}}}}},
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(controllerName), Type: &ast.StructType{Fields: &ast.FieldList{}}}}},
		&ast.FuncDecl{Name: ast.NewIdent("New" + exportedIdentifier(version)), Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent(controllerName)}}}}}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ast.NewIdent(controllerName)}}}}}}},
	)
	return formatGeneratedFile(file)
}

func renderControllerMethods(apis []API, packageName string) ([]byte, error) {
	if len(apis) == 0 {
		return nil, fmt.Errorf("controller implementation has no API methods")
	}
	sort.Slice(apis, func(left, right int) bool { return apis[left].Name < apis[right].Name })
	api := apis[0]
	imports := map[string]string{
		"context": "context", api.ImportPath: api.Package,
	}
	if err := validateImportNames(imports); err != nil {
		return nil, err
	}
	file := &ast.File{Name: ast.NewIdent(packageName), Decls: []ast.Decl{importDeclaration(imports)}}
	controllerName := "Controller" + exportedIdentifier(api.Version)
	for _, api := range apis {
		file.Decls = append(file.Decls, controllerMethodDecl(api, controllerName))
	}
	return formatImplementationFile(file)
}

func controllerMethodDecl(api API, controllerName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent(controllerName)}}}},
		Name: ast.NewIdent(api.Name),
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent("ctx")}, Type: selector("context", "Context")},
			{Names: []*ast.Ident{ast.NewIdent("req")}, Type: &ast.StarExpr{X: selector(api.Package, api.RequestType)}},
		}}, Results: &ast.FieldList{List: []*ast.Field{
			{Type: &ast.StarExpr{X: selector(api.Package, api.ResponseType)}}, {Type: ast.NewIdent("error")},
		}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{
				&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: selector(api.Package, api.ResponseType)}},
				ast.NewIdent("nil"),
			}},
		}},
	}
}

// importDeclaration receives package path -> package name. The name is kept
// only for collision validation; imports are intentionally emitted without
// aliases so generated code follows normal Go package conventions.
func importDeclaration(imports map[string]string) *ast.GenDecl {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	specs := make([]ast.Spec, 0, len(paths))
	for _, path := range paths {
		spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconvQuote(path)}}
		specs = append(specs, spec)
	}
	return &ast.GenDecl{Tok: token.IMPORT, Specs: specs}
}

func validateImportNames(imports map[string]string) error {
	seen := make(map[string]string, len(imports))
	for path, name := range imports {
		if previous, exists := seen[name]; exists && previous != path {
			return fmt.Errorf("generated imports %q and %q both use package name %q; aliases are disabled", previous, path, name)
		}
		seen[name] = path
	}
	return nil
}

func cleanStaleGeneratedContracts(project Project, destination string, expected map[string]struct{}, result *Result, dryRun bool) error {
	root := project.Resolve(destination)
	relativeRoot, err := filepath.Rel(project.Root, root)
	if err != nil || relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(filepath.Separator)) {
		return fmt.Errorf("--clean destination must be inside the project root: %s", destination)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), "_generated.go") {
			return nil
		}
		if _, exists := expected[filepath.Clean(path)]; exists {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !generatedFile(content) {
			return nil
		}
		relative, err := filepath.Rel(project.Root, path)
		if err != nil {
			return err
		}
		if dryRun {
			result.add("DELETE", relative, "dry-run; gx-owned stale contract")
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("delete stale generated file %s: %w", relative, err)
		}
		result.add("DELETE", relative, "gx-owned stale contract")
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("scan stale generated files: %w", err)
	}
	return nil
}

func selector(packageName, name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ast.NewIdent(packageName), Sel: ast.NewIdent(name)}
}

func formatGeneratedFile(file *ast.File) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString(generatedHeader)
	builder.WriteString("\n\n")
	formatted, err := formatGoFile(file)
	if err != nil {
		return nil, err
	}
	builder.Write(formatted)
	builder.WriteByte('\n')
	return []byte(builder.String()), nil
}

func formatImplementationFile(file *ast.File) ([]byte, error) {
	return formatGoFile(file)
}

func formatGoFile(file *ast.File) ([]byte, error) {
	var builder strings.Builder
	if err := format.Node(&builder, token.NewFileSet(), file); err != nil {
		return nil, err
	}
	builder.WriteString("\n")
	return []byte(builder.String()), nil
}

func goPackageName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func exportedIdentifier(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func toSnake(value string) string {
	var builder strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			builder.WriteByte('_')
		}
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

func strconvQuote(value string) string {
	return fmt.Sprintf("%q", value)
}
