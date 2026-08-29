package router_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/router"
)

type definitionRequest struct {
	g.Meta `path:"/definition" method:"GET"`
}

type definitionController struct{}

func (*definitionController) Get(context.Context, *definitionRequest) error { return nil }

func TestDefinitionSeparatesMetadataAndRuntime(t *testing.T) {
	routes, err := router.ScanController(&definitionController{})
	if err != nil {
		t.Fatal(err)
	}
	route := routes[0]
	if route.Metadata.Path != "/definition" || route.Metadata.Method != "GET" {
		t.Fatalf("metadata = %#v", route.Metadata)
	}
	if route.Runtime.Binder == nil || !route.Runtime.MethodValue.IsValid() {
		t.Fatal("route runtime is incomplete")
	}
	if route.Runtime.MethodValue.Type() != reflect.TypeOf((func(context.Context, *definitionRequest) error)(nil)) {
		t.Fatalf("runtime method type = %s", route.Runtime.MethodValue.Type())
	}
}
