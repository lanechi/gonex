package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

const standardVersion = "v1"

type standardResource struct {
	Module  string
	Version string
	File    string
}

func renderStandardService(discovery Discovery) (Rendered, error) {
	project, options := discovery.Project, discovery.Options
	module, err := normalizeResourceName(options.Name)
	if err != nil {
		return Rendered{}, err
	}
	rendered := Rendered{Discovery: discovery}
	demoModel, err := demoModelOutput(project)
	if err != nil {
		return Rendered{}, err
	}
	if demoModel != nil {
		rendered.Outputs = append(rendered.Outputs, *demoModel)
	}
	resource := standardResource{Module: module, Version: standardVersion, File: module}
	servicePath := filepath.Join(project.Resolve(options.Destination), module+".go")
	modelImportPath := demoModelImportPath(project)
	rendered.Outputs = append(rendered.Outputs, shared.Output{Path: servicePath, Content: renderStandardServiceSource(resource, modelImportPath), Mode: shared.OutputReplacing})
	logicPath := filepath.Join(project.Resolve(options.Source), module, module+".go")
	rendered.Outputs = append(rendered.Outputs, shared.Output{Path: logicPath, Content: renderStandardLogic(resource, project.ModulePath, modelImportPath), Mode: shared.OutputDeveloperOwned, Label: "Logic implementation"})
	aggregator, err := logicAggregatorOutput(project, options.Source, map[string][]LogicMethod{module: nil})
	if err != nil {
		return Rendered{}, err
	}
	rendered.Outputs = append(rendered.Outputs, aggregator)
	return rendered, nil
}

func normalizeResourceName(value string) (string, error) {
	value = filepath.Base(filepath.Clean(value))
	if !validIdentifier(value) {
		return "", fmt.Errorf("invalid resource name %q", value)
	}
	return value, nil
}

func renderStandardServiceSource(resource standardResource, modelImportPath string) []byte {
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
