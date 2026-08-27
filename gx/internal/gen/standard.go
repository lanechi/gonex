package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const standardVersion = "v1"

type standardResource struct {
	Module  string
	Version string
	File    string
}

// standardCRUDAPIs describes the small, field-agnostic CRUD contract created
// by `ctrl <name>`. Resource-specific fields can be added after creation.
func standardCRUDAPIs(project Project, resource standardResource, sourceDir string) []API {
	apiName := exportedIdentifier(resource.File)
	packageName := apiPackageName(sourceDir)
	basePath := "/" + resource.File
	resourcePath := project.ModulePath + "/" + filepath.ToSlash(sourceDir)
	source := filepath.ToSlash(filepath.Join(sourceDir, resource.File+".go"))
	return []API{
		{Module: resource.Module, Version: resource.Version, Package: packageName, Name: "Create" + apiName, ImportPath: resourcePath, RequestType: "Create" + apiName + "Req", ResponseType: "Create" + apiName + "Res", Source: source, Path: basePath, Method: "POST", Tags: []string{exportedIdentifier(resource.Module)}, Summary: "Create " + resource.File},
		{Module: resource.Module, Version: resource.Version, Package: packageName, Name: "Update" + apiName, ImportPath: resourcePath, RequestType: "Update" + apiName + "Req", ResponseType: "Update" + apiName + "Res", Source: source, Path: basePath + "/:id", Method: "PUT", Tags: []string{exportedIdentifier(resource.Module)}, Summary: "Update " + resource.File},
		{Module: resource.Module, Version: resource.Version, Package: packageName, Name: "Delete" + apiName, ImportPath: resourcePath, RequestType: "Delete" + apiName + "Req", ResponseType: "Delete" + apiName + "Res", Source: source, Path: basePath + "/:id", Method: "DELETE", Tags: []string{exportedIdentifier(resource.Module)}, Summary: "Delete " + resource.File},
		{Module: resource.Module, Version: resource.Version, Package: packageName, Name: "GetOne" + apiName, ImportPath: resourcePath, RequestType: "GetOne" + apiName + "Req", ResponseType: "GetOne" + apiName + "Res", Source: source, Path: basePath + "/:id", Method: "GET", Tags: []string{exportedIdentifier(resource.Module)}, Summary: "Get one " + resource.File},
		{Module: resource.Module, Version: resource.Version, Package: packageName, Name: "GetList" + apiName, ImportPath: resourcePath, RequestType: "GetList" + apiName + "Req", ResponseType: "GetList" + apiName + "Res", Source: source, Path: basePath, Method: "GET", Tags: []string{exportedIdentifier(resource.Module)}, Summary: "Get " + resource.File + " list"},
	}
}

func generateStandardControllers(project Project, options ControllerOptions) (Result, error) {
	var result Result
	explicitSource := options.Source != ""
	if options.Source == "" {
		options.Source = defaultAPISource
	}
	if options.Destination == "" {
		options.Destination = defaultControllerDest
	}
	var resource standardResource
	var err error
	if explicitSource {
		resource, err = resourceForDirectory(project, options.Source, options.Name)
	} else {
		resource, err = parseStandardResource(options.Name)
	}
	if err != nil {
		return result, err
	}
	apiDirectory := options.Source
	if !explicitSource {
		apiDirectory = filepath.Join(options.Source, resource.Module, resource.Version)
	}
	apis := standardCRUDAPIs(project, resource, apiDirectory)
	apiRootDir := options.Source
	if explicitSource {
		apiRootDir = filepath.Dir(filepath.Dir(options.Source))
	}
	apiRootPath := filepath.Join(project.Resolve(apiRootDir), resource.Module, resource.Module+".go")
	apiPath := filepath.Join(project.Resolve(apiDirectory), resource.File+".go")
	apiSource := renderStandardAPI(apiPackageName(apiDirectory), apis)
	if err := writeDeveloperOwned(project, &result, apiPath, apiSource, "API definition", options.DryRun); err != nil {
		return result, err
	}
	contractAPIs, err := scanStandardControllerAPIs(project, apiDirectory, resource, apis)
	if err != nil {
		return result, err
	}

	packageName := goPackageName(resource.Module)
	generatedPath := filepath.Join(project.Resolve(options.Destination), resource.Module, resource.Module+".go")
	contractGroups := map[string][]API{resource.Version: contractAPIs}
	if !explicitSource {
		if allAPIs, scanErr := scanAPIs(project, options.Source); scanErr == nil {
			contractGroups = make(map[string][]API)
			for _, api := range allAPIs {
				if api.Module == resource.Module {
					contractGroups[api.Version] = append(contractGroups[api.Version], api)
				}
			}
			if len(contractGroups) == 0 {
				contractGroups[resource.Version] = contractAPIs
			}
		}
	}
	if err := writeForced(project, &result, apiRootPath, renderAPIContracts(contractGroups, resource.Module, resource.Module), options.DryRun); err != nil {
		return result, err
	}
	contract, err := renderControllerContracts(project, contractGroups, packageName, resource.Module)
	if err != nil {
		return result, err
	}
	if err := writePlanned(project, &result, generatedPath, contract, options.DryRun); err != nil {
		return result, err
	}
	constructorPath := filepath.Join(project.Resolve(options.Destination), resource.Module, resource.Module+"_new.go")
	constructor, err := renderControllerConstructors(packageName, contractGroups)
	if err != nil {
		return result, err
	}
	if err := writePlanned(project, &result, constructorPath, constructor, options.DryRun); err != nil {
		return result, err
	}

	implementationPath := filepath.Join(project.Resolve(options.Destination), resource.Module, resource.Module+"_"+resource.Version+"_"+toSnake(resource.File)+".go")
	implementation, err := renderControllerMethods(apis, packageName)
	if err != nil {
		return result, err
	}
	if err := writeDeveloperOwned(project, &result, implementationPath, implementation, "controller implementation", options.DryRun); err != nil {
		return result, err
	}
	return result, nil
}

// scanStandardControllerAPIs returns every API in the generated resource's
// module/version directory. Named generation must rebuild the controller
// contract from the complete directory so adding one API does not discard
// methods generated by earlier invocations.
func scanStandardControllerAPIs(project Project, apiDirectory string, resource standardResource, generated []API) ([]API, error) {
	resourceDirectory := project.Resolve(apiDirectory)
	entries, err := os.ReadDir(resourceDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return generated, nil
		}
		return nil, fmt.Errorf("read API directory %s: %w", apiDirectory, err)
	}
	apiRoot := filepath.Dir(filepath.Dir(resourceDirectory))
	filtered := make([]API, 0, len(generated))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(resourceDirectory, entry.Name())
		apis, parseErr := parseAPIFile(project, apiRoot, path)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, api := range apis {
			if api.Module == resource.Module && api.Version == resource.Version {
				filtered = append(filtered, api)
			}
		}
	}
	generatedNames := make(map[string]struct{}, len(generated))
	for _, api := range generated {
		generatedNames[api.Name] = struct{}{}
	}
	remaining := filtered[:0]
	for _, api := range filtered {
		if _, ok := generatedNames[api.Name]; !ok {
			remaining = append(remaining, api)
		}
	}
	filtered = append(remaining, generated...)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no API request definitions found under %s", apiDirectory)
	}
	sort.Slice(filtered, func(left, right int) bool {
		return filtered[left].Name < filtered[right].Name
	})
	return filtered, nil
}

func generateStandardService(project Project, options ServiceOptions) (Result, error) {
	var result Result
	module, err := normalizeResourceName(options.Name)
	if err != nil {
		return result, err
	}
	if options.Source == "" {
		options.Source = defaultLogicSource
	}
	if options.Destination == "" {
		options.Destination = defaultServiceDest
	}
	if err := ensureDemoModel(project, &result, options.DryRun); err != nil {
		return result, err
	}
	resource := standardResource{Module: module, Version: standardVersion, File: module}
	servicePath := filepath.Join(project.Resolve(options.Destination), module+".go")
	modelImportPath := demoModelImportPath(project)
	serviceSource := renderStandardService(resource, modelImportPath)
	if err := writeReplacing(project, &result, servicePath, serviceSource, options.DryRun); err != nil {
		return result, err
	}

	logicPath := filepath.Join(project.Resolve(options.Source), module, module+".go")
	logicSource := renderStandardLogic(resource, project.ModulePath, modelImportPath)
	if err := writeDeveloperOwned(project, &result, logicPath, logicSource, "Logic implementation", options.DryRun); err != nil {
		return result, err
	}
	if err := syncLogicAggregator(project, options.Source, map[string][]LogicMethod{module: nil}, &result, options.DryRun); err != nil {
		return result, err
	}
	return result, nil
}

func parseStandardResource(value string) (standardResource, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return standardResource{}, fmt.Errorf("resource name is required")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/")
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		if !validIdentifier(parts[0]) || strings.Trim(parts[0], "_") == "" {
			return standardResource{}, fmt.Errorf("invalid resource name %q: use a Go identifier or module/version/file path", value)
		}
		module := toSnake(parts[0])
		return standardResource{Module: module, Version: standardVersion, File: module}, nil
	}
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return standardResource{}, fmt.Errorf("invalid resource path %q: use module/version/file, for example user/v1/newapi", value)
	}
	if !validIdentifier(parts[0]) || strings.Trim(parts[0], "_") == "" {
		return standardResource{}, fmt.Errorf("invalid resource module %q", parts[0])
	}
	if !validVersion(parts[1]) {
		return standardResource{}, fmt.Errorf("invalid resource version %q: use v1 or v2", parts[1])
	}
	if !validIdentifier(parts[2]) || strings.Trim(parts[2], "_") == "" {
		return standardResource{}, fmt.Errorf("invalid resource file %q: use a Go identifier", parts[2])
	}
	return standardResource{Module: toSnake(parts[0]), Version: parts[1], File: toSnake(parts[2])}, nil
}

func normalizeResourceName(value string) (string, error) {
	resource, err := parseStandardResource(value)
	if err != nil {
		return "", err
	}
	return resource.Module, nil
}

func projectRelativePath(project Project, path string) (string, error) {
	resolved := project.Resolve(path)
	relative, err := filepath.Rel(project.Root, resolved)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must be inside the project root: %s", path)
	}
	if relative == "." {
		return "", nil
	}
	return filepath.ToSlash(relative), nil
}

func resourceForDirectory(project Project, directory, name string) (standardResource, error) {
	relative, err := projectRelativePath(project, directory)
	if err != nil {
		return standardResource{}, err
	}
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	if len(parts) < 2 {
		return standardResource{}, fmt.Errorf("--dir must end with module/version, for example api/user/v1: %s", directory)
	}
	module, version := parts[len(parts)-2], parts[len(parts)-1]
	if !validIdentifier(module) || strings.Trim(module, "_") == "" {
		return standardResource{}, fmt.Errorf("--dir has invalid module directory %q", module)
	}
	if !validVersion(version) {
		return standardResource{}, fmt.Errorf("--dir must end with a version such as v1: %s", directory)
	}
	resource, err := parseStandardResource(name)
	if err != nil {
		return standardResource{}, err
	}
	if strings.Contains(strings.ReplaceAll(strings.Trim(name, "/"), "\\", "/"), "/") && resource.Module != toSnake(module) {
		return standardResource{}, fmt.Errorf("resource path module %q does not match --dir module %q", resource.Module, module)
	}
	return standardResource{Module: toSnake(module), Version: version, File: resource.File}, nil
}

func apiPackageName(directory string) string {
	return filepath.Base(filepath.Clean(directory))
}

func renderStandardAPI(packageName string, apis []API) []byte {
	var builder strings.Builder
	fmt.Fprintf(&builder, "package %s\n\nimport %q\n\n", packageName, "github.com/lanechi/gonex/g")
	for _, api := range apis {
		fmt.Fprintf(&builder, "type %s struct {\n", api.RequestType)
		fmt.Fprintf(&builder, "\tg.Meta `path:\"%s\" method:\"%s\" tags:\"%s\" summary:\"%s\"`\n", api.Path, strings.ToLower(api.Method), strings.Join(api.Tags, ","), api.Summary)
		if strings.HasPrefix(api.Name, "Update") || strings.HasPrefix(api.Name, "Delete") || strings.HasPrefix(api.Name, "GetOne") {
			builder.WriteString("\tID int64 `path:\"id\" binding:\"required\" validate:\"gt=0\"`\n")
		}
		builder.WriteString("}\n\n")
		fmt.Fprintf(&builder, "type %s struct{}\n\n", api.ResponseType)
	}
	return []byte(builder.String())
}

// renderAPIContracts places the versioned API interfaces in the module-level
// API package, matching the public layout used by the demo project.
func renderAPIContracts(byVersion map[string][]API, packageName, module string) []byte {
	versions := make([]string, 0, len(byVersion))
	imports := make(map[string]string)
	for version, apis := range byVersion {
		versions = append(versions, version)
		for _, api := range apis {
			imports[api.ImportPath] = api.Package
		}
	}
	sort.Strings(versions)
	var builder strings.Builder
	fmt.Fprintf(&builder, "package %s\n\n", packageName)
	if len(imports) > 0 {
		builder.WriteString("import (\n\t\"context\"\n\n")
		paths := make([]string, 0, len(imports))
		for path := range imports {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			fmt.Fprintf(&builder, "\t%s %q\n", imports[path], path)
		}
		builder.WriteString(")\n\n")
	}
	for _, version := range versions {
		apis := append([]API(nil), byVersion[version]...)
		sort.Slice(apis, func(left, right int) bool { return apis[left].Name < apis[right].Name })
		fmt.Fprintf(&builder, "type I%s%s interface {\n", exportedIdentifier(module), exportedIdentifier(version))
		for _, api := range apis {
			fmt.Fprintf(&builder, "\t%s(ctx context.Context, req *%s.%s) (*%s.%s, error)\n", api.Name, api.Package, api.RequestType, api.Package, api.ResponseType)
		}
		builder.WriteString("}\n\n")
	}
	return []byte(builder.String())
}

// renderStandardService deliberately keeps request and response models
// independent from the API layer. The demo model gives generated Logic and
// Service code a concrete type that can later be replaced by business models.
func renderStandardService(resource standardResource, modelImportPath string) []byte {
	moduleName := exportedIdentifier(resource.Module)
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s\n\npackage service\n\nimport (\n\t\"context\"\n\n\t%q\n)\n\n", generatedHeader, modelImportPath)
	fmt.Fprintf(&builder, "type I%s interface {\n", moduleName)
	for _, operation := range []string{"Create", "Update", "Delete", "GetOne", "GetList"} {
		responseModel := "TestModel"
		if operation == "GetList" {
			responseModel = "TestModelList"
		}
		fmt.Fprintf(&builder, "\t%s(ctx context.Context, req *model.TestModel) (*model.%s, error)\n", operation, responseModel)
	}
	builder.WriteString("}\n\n")
	fmt.Fprintf(&builder, "var local%s I%s\n\n", moduleName, moduleName)
	fmt.Fprintf(&builder, "func %s() I%s {\n\tif local%s == nil {\n\t\tpanic(\"gx: %s service is not registered\")\n\t}\n\treturn local%s\n}\n\n", moduleName, moduleName, moduleName, resource.Module, moduleName)
	fmt.Fprintf(&builder, "func Register%s(implementation I%s) {\n\tlocal%s = implementation\n}\n", moduleName, moduleName, moduleName)
	return []byte(builder.String())
}

func renderStandardLogic(resource standardResource, modulePath, modelImportPath string) []byte {
	moduleName := exportedIdentifier(resource.Module)
	var builder strings.Builder
	fmt.Fprintf(&builder, "package %s\n\nimport (\n\t\"context\"\n\n\t%q\n\t%q\n)\n\ntype s%s struct{}\n\nfunc init() {\n\tservice.Register%s(New())\n}\n\nfunc New() service.I%s {\n\treturn &s%s{}\n}\n\n", resource.Module, modelImportPath, modulePath+"/internal/service", moduleName, moduleName, moduleName, moduleName)
	for _, operation := range []string{"Create", "Update", "Delete", "GetOne", "GetList"} {
		responseModel := "TestModel"
		if operation == "GetList" {
			responseModel = "TestModelList"
		}
		fmt.Fprintf(&builder, "func (*s%s) %s(_ context.Context, _ *model.TestModel) (*model.%s, error) {\n\treturn &model.%s{}, nil\n}\n\n", moduleName, operation, responseModel, responseModel)
	}
	return []byte(builder.String())
}
