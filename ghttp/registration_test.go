package ghttp

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/router"
)

func TestRouteHandlerLimitIsValidatedBeforeBatchRegistration(t *testing.T) {
	server := NewServer()
	middlewareCount := ginAbortIndex - len(server.engine.Handlers) - 1
	if middlewareCount <= 0 {
		t.Fatalf("unexpected global handler count: %d", len(server.engine.Handlers))
	}
	middleware := make([]Middleware, middlewareCount)
	for index := range middleware {
		middleware[index] = func(context *gin.Context) { context.Next() }
	}
	if err := server.Route("GET", "/handler-limit/z").Use(middleware...); err != nil {
		t.Fatal(err)
	}
	routes := []router.Definition{
		{Metadata: router.RouteMetadata{Method: "GET", Path: "/handler-limit/a"}, Runtime: router.RouteRuntime{Binder: &router.Binder{}}},
		{Metadata: router.RouteMetadata{Method: "GET", Path: "/handler-limit/z"}, Runtime: router.RouteRuntime{Binder: &router.Binder{}}},
	}
	if err := server.registerRouteDefinitions(routes, nil); err == nil || !strings.Contains(err.Error(), "handlers") {
		t.Fatalf("registration error = %v", err)
	}
	for _, route := range server.engine.Routes() {
		if strings.HasPrefix(route.Path, "/handler-limit/") {
			t.Fatalf("failed batch left Gin route %s %s", route.Method, route.Path)
		}
	}
	if got := server.registry.List(); len(got) != 0 {
		t.Fatalf("failed batch updated registry: %#v", got)
	}
}
