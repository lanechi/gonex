package g_test

import (
	"context"
	"testing"

	"github.com/lanechi/gonex/g"
)

func TestContextOutsideHTTPRequestReturnsNil(t *testing.T) {
	if got := g.Ctx(context.Background()); got != nil {
		t.Fatalf("g.Context(context.Background()) = %#v, want nil", got)
	}
}

func TestNamedServersAreIndependentAndStable(t *testing.T) {
	defaultServer := g.Server()
	if defaultServer != g.Server("default") || defaultServer != g.Server("") {
		t.Fatal("default server should be shared by the default names")
	}

	apiName := t.Name() + "/api"
	adminName := t.Name() + "/admin"
	api := g.Server(apiName)
	admin := g.Server(adminName)
	if api == admin || api == defaultServer {
		t.Fatal("named servers should be independent from one another and the default server")
	}
	if api != g.Server(apiName) || admin != g.Server(adminName) {
		t.Fatal("named server lookup should be stable")
	}
	if api.Name() != apiName || admin.Name() != adminName {
		t.Fatalf("named server names: api=%q admin=%q", api.Name(), admin.Name())
	}
	if api.Engine() == admin.Engine() || api.HTTPServer() == admin.HTTPServer() {
		t.Fatal("named servers should not share HTTP state")
	}
}
