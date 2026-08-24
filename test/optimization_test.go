package ghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type atomicParamRequest struct {
	g.Meta `       path:"/atomic/:id" method:"get"`
	ID     string `path:"id"`
}

type atomicCatchallRequest struct {
	g.Meta `       path:"/atomic/*path" method:"get"`
	Path   string `path:"path"`
}

type atomicConflictController struct{}

func (*atomicConflictController) Catchall(context.Context, *atomicCatchallRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

func (*atomicConflictController) Param(context.Context, *atomicParamRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

func TestBatchRouteValidationDoesNotPartiallyMutateGin(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&atomicConflictController{}); err == nil ||
		!strings.Contains(err.Error(), "route table conflict") {
		t.Fatalf("conflicting batch bind error=%v", err)
	}
	for _, route := range server.Engine().Routes() {
		if strings.HasPrefix(route.Path, "/atomic") {
			t.Fatalf("failed batch left Gin route registered: %#v", route)
		}
	}
	if len(server.Routes()) != 0 {
		t.Fatalf("failed batch left framework routes: %#v", server.Routes())
	}
}

type committedErrorRequest struct {
	g.Meta `path:"/committed-error" method:"get"`
}

type committedErrorController struct{}

func (*committedErrorController) Write(ctx context.Context, _ *committedErrorRequest) (*helloResponse, error) {
	ghttp.FromContext(ctx).String(http.StatusOK, "partial")
	return nil, errors.New("controller error after response")
}

func TestCommittedResponseIsNotAppendedWithErrorEnvelope(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&committedErrorController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/committed-error", nil)
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "partial" {
		t.Fatalf("committed response was changed: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestNamedServer(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithName("compat"), ghttp.WithLogger(&recordingLogger{}))
	if server.Name() != "compat" {
		t.Fatalf("server name = %q", server.Name())
	}
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"message":"hello"`) {
		t.Fatalf("named server response: status=%d body=%s", response.Code, response.Body.String())
	}
}
