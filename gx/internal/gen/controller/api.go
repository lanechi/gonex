package controller

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func scanAPIs(project Project, sourceDir string) ([]API, error) {
	root := project.Resolve(sourceDir)
	sourceRoot := apiSourceRoot(project, root)
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("API source directory %s does not exist", sourceDir)
		}
		return nil, fmt.Errorf("scan API directory: %w", err)
	}
	sort.Strings(files)
	var result []API
	for _, path := range files {
		apis, parseErr := parseAPIFile(project, sourceRoot, path)
		if parseErr != nil {
			return nil, parseErr
		}
		result = append(result, apis...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no API request definitions found under %s", sourceDir)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Module + "/" + result[i].Version + "/" + result[i].Name
		right := result[j].Module + "/" + result[j].Version + "/" + result[j].Name
		return left < right
	})
	seenRoutes := make(map[string]string)
	for _, api := range result {
		// API modules are bound to different controller groups by the
		// application, so identical paths in different module/version
		// packages do not conflict with one another.
		key := api.Module + "/" + api.Version + " " + strings.ToUpper(api.Method) + " " + api.Path
		if previous, exists := seenRoutes[key]; exists {
			return nil, fmt.Errorf("duplicate API route %s in %s and %s", key, previous, api.Source)
		}
		seenRoutes[key] = api.Source
	}
	return result, nil
}

// apiSourceRoot returns the module root when --dir points directly at an
// API's <module>/<version> directory. The normal scanner receives api (or a
// custom API root), while directory-scoped generation receives api/user/v1;
// both forms must produce the same module/version metadata and import paths.
func apiSourceRoot(project Project, root string) string {
	relative, err := filepath.Rel(project.Root, root)
	if err != nil {
		return root
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(relative)), "/")
	if len(parts) < 3 {
		return root
	}
	module := parts[len(parts)-2]
	version := parts[len(parts)-1]
	if !validIdentifier(module) || !validVersion(version) {
		return root
	}
	return filepath.Dir(filepath.Dir(root))
}

func parseAPIFile(project Project, sourceRoot, path string) ([]API, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse API file %s: %w", path, err)
	}
	relative, err := filepath.Rel(project.Root, path)
	if err != nil {
		return nil, err
	}
	fromSource, err := filepath.Rel(sourceRoot, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	importRelative, err := filepath.Rel(project.Root, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(filepath.ToSlash(fromSource), "/")
	if len(parts) != 2 || parts[0] == "." {
		// Source roots may contain package-level helpers such as
		// api/user/user.go. Only files in <module>/<version> are API inputs.
		return nil, nil
	}
	module, version := parts[0], parts[1]
	if !validIdentifier(module) || !validVersion(version) {
		return nil, fmt.Errorf("API file %s has invalid module/version directory %q/%q", relative, module, version)
	}
	types := make(map[string]*ast.TypeSpec)
	for _, declaration := range file.Decls {
		genDecl, ok := declaration.(*ast.GenDecl)
		if !ok || genDecl.Tok.String() != "type" {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			types[typeSpec.Name.Name] = typeSpec
		}
	}
	var result []API
	for typeName, typeSpec := range types {
		if !strings.HasSuffix(typeName, "Req") || typeSpec.Assign.IsValid() {
			continue
		}
		name := strings.TrimSuffix(typeName, "Req")
		response, ok := types[name+"Res"]
		if !ok {
			return nil, fmt.Errorf("%s: %s exists but %s not found", relative, typeName, name+"Res")
		}
		_ = response
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be a struct", relative, typeName)
		}
		metadata, ok := routeMetadata(structType)
		if !ok {
			return nil, fmt.Errorf("%s: %s must embed g.Meta with path and method tags", relative, typeName)
		}
		result = append(result, API{
			Module: module, Version: version, Package: file.Name.Name, Name: name,
			ImportPath:  project.ModulePath + "/" + filepath.ToSlash(importRelative),
			RequestType: typeName, ResponseType: name + "Res", Source: relative,
			Path: metadata.Path, Method: metadata.Method, Tags: metadata.Tags,
			Summary: metadata.Summary, Description: metadata.Description, OperationID: metadata.OperationID,
		})
	}
	return result, nil
}

type routeMeta struct {
	Path        string
	Method      string
	Tags        []string
	Summary     string
	Description string
	OperationID string
}

func routeMetadata(structType *ast.StructType) (routeMeta, bool) {
	for _, field := range structType.Fields.List {
		if len(field.Names) != 0 || field.Tag == nil || !isMetaType(field.Type) {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return routeMeta{}, false
		}
		tags := reflect.StructTag(raw)
		path, method := strings.TrimSpace(tags.Get("path")), strings.ToUpper(strings.TrimSpace(tags.Get("method")))
		if path == "" || method == "" {
			return routeMeta{}, false
		}
		if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, " \t\r\n") {
			return routeMeta{}, false
		}
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE":
		default:
			return routeMeta{}, false
		}
		return routeMeta{
			Path: path, Method: method, Tags: splitTag(tags.Get("tags")),
			Summary: strings.TrimSpace(tags.Get("summary")), Description: strings.TrimSpace(tags.Get("description")),
			OperationID: strings.TrimSpace(tags.Get("operationId")),
		}, true
	}
	return routeMeta{}, false
}

func isMetaType(expression ast.Expr) bool {
	switch typeExpr := expression.(type) {
	case *ast.SelectorExpr:
		return typeExpr.Sel.Name == "Meta"
	case *ast.StarExpr:
		return isMetaType(typeExpr.X)
	default:
		return false
	}
}

func splitTag(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

func validIdentifier(value string) bool {
	if value == "" || !((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z') || value[0] == '_') {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_') {
			return false
		}
	}
	return true
}

func validVersion(value string) bool {
	return len(value) > 1 && value[0] == 'v' && validDigits(value[1:])
}

func validDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
