package test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanechi/gonex/gx/internal/gen"
)

func TestGeneratorsCreateControllerServiceAndLogic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(root, "api/user/v1/user.go"), `package v1

import "github.com/lanechi/gonex/g"

type CreateReq struct {
	g.Meta `+"`path:\"/users\" method:\"post\" summary:\"create user\"`"+`
}

type CreateRes struct{}
`)
	writeFile(t, filepath.Join(root, "internal/logic/user/user.go"), `package user

import "context"

type sUser struct{}

func (*sUser) Ping(context.Context) error { return nil }
`)

	project, err := gen.DiscoverProject(filepath.Join(root, "api", "user", "v1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateControllers(project, gen.ControllerOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateServices(project, gen.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}

	for _, relative := range []string{
		"internal/controller/user/user.go",
		"internal/controller/user/user_new.go",
		"internal/controller/user/user_v1_user.go",
		"internal/service/user.go",
		"internal/logic/logic.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments); err != nil {
			t.Fatalf("generated %s is not valid Go: %v", relative, err)
		}
	}
	service, _ := os.ReadFile(filepath.Join(root, "internal/service/user.go"))
	if !strings.Contains(string(service), "func User() IUser") {
		t.Fatalf("service registration was not generated: %s", service)
	}
}

func TestGeneratedAPIUsesLastDirectoryAsPackageName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	project, err := gen.DiscoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateControllers(project, gen.ControllerOptions{Name: "public/v1/page"}); err != nil {
		t.Fatal(err)
	}
	apiRoot, err := os.ReadFile(filepath.Join(root, "api/public/public.go"))
	if err != nil {
		t.Fatalf("module API package was not created: %v", err)
	}
	if !strings.Contains(string(apiRoot), "type IPublicV1 interface") {
		t.Fatalf("module API interface was not generated: %s", apiRoot)
	}
	api, err := os.ReadFile(filepath.Join(root, "api/public/v1/page.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(api), "package v1\n") {
		t.Fatalf("generated API package should match the last directory: %s", api)
	}
	for _, relative := range []string{"internal/controller/public/public.go", "internal/controller/public/public_new.go"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("controller structure missing %s: %v", relative, err)
		}
	}
	controller, _ := os.ReadFile(filepath.Join(root, "internal/controller/public/public.go"))
	if strings.Contains(string(controller), "IPublicV1") {
		t.Fatalf("controller contract should live in the API package: %s", controller)
	}
	constructor, _ := os.ReadFile(filepath.Join(root, "internal/controller/public/public_new.go"))
	if !strings.Contains(string(constructor), "type ControllerV1 struct") || !strings.Contains(string(constructor), "func NewV1()") {
		t.Fatalf("controller type and constructor were not generated together: %s", constructor)
	}
}

func TestServiceGenerationReplacesExistingServiceFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(root, "internal/logic/user/user.go"), `package user

import "context"

type sUser struct{}

func (*sUser) Ping(context.Context) error { return nil }
`)
	servicePath := filepath.Join(root, "internal/service/user.go")
	writeFile(t, servicePath, "package service\n\n// stale service\n")
	project, err := gen.DiscoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateServices(project, gen.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	service, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(service), "stale service") || !strings.Contains(string(service), "type IUser interface") {
		t.Fatalf("existing service was not replaced: %s", service)
	}
}

func TestServiceGeneratorNormalizesModuleNames(t *testing.T) {
	tests := []struct {
		module string
		name   string
	}{
		{module: "user_profile", name: "UserProfile"},
		{module: "userProfile", name: "UserProfile"},
		{module: "User_Profile", name: "UserProfile"},
		{module: "User_profile", name: "UserProfile"},
		// There is no word boundary to infer in these names, so keep the
		// existing single-word result.
		{module: "userprofile", name: "Userprofile"},
		{module: "Userprofile", name: "Userprofile"},
	}
	for _, test := range tests {
		t.Run(test.module, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
			writeFile(t, filepath.Join(root, "internal/logic", test.module, "logic.go"), `package logic

import "context"

func Ping(context.Context) error { return nil }
`)
			project, err := gen.DiscoverProject(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := gen.GenerateServices(project, gen.ServiceOptions{}); err != nil {
				t.Fatal(err)
			}
			service, err := os.ReadFile(filepath.Join(root, "internal/service", test.module+".go"))
			if err != nil {
				t.Fatal(err)
			}
			content := string(service)
			for _, expected := range []string{
				"type I" + test.name + " interface",
				"func " + test.name + "() I" + test.name,
				"func Register" + test.name + "(implementation I" + test.name,
			} {
				if !strings.Contains(content, expected) {
					t.Errorf("generated service for %s does not contain %q:\n%s", test.module, expected, content)
				}
			}
		})
	}
}

func TestServiceGeneratorPreservesAliasedImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(root, "internal/logic/order/order.go"), `package order

import (
	"context"
	dto "example.com/contracts/models"
)

type sOrder struct{}
type sRefund struct{}

func (*sOrder) Create(context.Context, dto.CreateRequest) error { return nil }
func (*sRefund) Apply(context.Context, dto.CreateRequest) error { return nil }
`)
	project, err := gen.DiscoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateServices(project, gen.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	service, err := os.ReadFile(filepath.Join(root, "internal/service/order.go"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(service)
	if !strings.Contains(content, `dto "example.com/contracts/models"`) {
		t.Fatalf("aliased import was not preserved: %s", content)
	}
	if !strings.Contains(content, "dto.CreateRequest") {
		t.Fatalf("aliased type was not preserved: %s", content)
	}
	for _, expected := range []string{
		"type IOrder interface",
		"func Order() IOrder",
		"func RegisterOrder(implementation IOrder)",
		"type IRefund interface",
		"func Refund() IRefund",
		"func RegisterRefund(implementation IRefund)",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("receiver-specific service was not generated (%q): %s", expected, content)
		}
	}
}

func TestGeneratorsPreserveDeveloperImplementationAndSupportDryRun(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(root, "api/user/v1/user.go"), `package v1

import "github.com/lanechi/gonex/g"

type CreateReq struct { g.Meta `+"`path:\"/users\" method:\"post\"`"+` }
type CreateRes struct{}
`)
	project, err := gen.DiscoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateControllers(project, gen.ControllerOptions{}); err != nil {
		t.Fatal(err)
	}
	implementation := filepath.Join(root, "internal/controller/user/user_v1_user.go")
	writeFile(t, implementation, "package user\n\n// developer implementation\n")
	before, _ := os.ReadFile(implementation)
	if _, err := gen.GenerateControllers(project, gen.ControllerOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(implementation)
	if string(before) != string(after) {
		t.Fatal("controller implementation was overwritten")
	}

	dryRoot := t.TempDir()
	writeFile(t, filepath.Join(dryRoot, "go.mod"), "module example.com/dry\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(dryRoot, "api/user/v1/user.go"), string(mustRead(t, filepath.Join(root, "api/user/v1/user.go"))))
	dryProject, err := gen.DiscoverProject(dryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateControllers(dryProject, gen.ControllerOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dryRoot, "internal/controller")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote controller output: %v", err)
	}
}

func TestControllerGeneratorInitializesAnyNamedResponseWithNew(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(root, "api/review/v1/review.go"), `package v1

import "github.com/lanechi/gonex/g"

type ListReq struct { g.Meta `+"`path:\"/reviews\" method:\"get\"`"+` }
type ListRes []string
`)
	project, err := gen.DiscoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenerateControllers(project, gen.ControllerOptions{}); err != nil {
		t.Fatal(err)
	}
	implementation, err := os.ReadFile(filepath.Join(root, "internal/controller/review/review_v1_review.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(implementation), "new(v1.ListRes)") {
		t.Fatalf("named slice response was not initialized with new: %s", implementation)
	}
}

func TestCanonicalDemoGenerationDryRun(t *testing.T) {
	project, err := gen.DiscoverProject(filepath.Join(repositoryRoot(), "examples", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []func() (gen.Result, error){
		func() (gen.Result, error) {
			return gen.GenerateControllers(project, gen.ControllerOptions{DryRun: true})
		},
		func() (gen.Result, error) { return gen.GenerateServices(project, gen.ServiceOptions{DryRun: true}) },
	} {
		output, err := result()
		if err != nil {
			t.Fatal(err)
		}
		for _, change := range output.Changes {
			if change.Kind != "SKIP" && !(change.Kind == "UPDATE" && strings.HasPrefix(change.Path, "internal/service/")) {
				t.Fatalf("canonical demo dry-run found a difference: %#v", change)
			}
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
