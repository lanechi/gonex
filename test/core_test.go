package ghttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/router"
)

func TestServerBindAndServe(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"message":"hello"`) ||
		!strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("response: status=%d body=%s", response.Code, response.Body.String())
	}
	routes := server.Routes()
	if len(routes) != 1 || routes[0].Method != "GET" || routes[0].Path != "/hello" {
		t.Fatalf("routes = %#v", routes)
	}
	if !strings.Contains(server.RouteTable(), "helloController.Hello") {
		t.Fatalf("route table = %s", server.RouteTable())
	}
}

func TestErrorOnlyControllerUsesEmptyDataObject(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&emptyController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/empty", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":{}`) {
		t.Fatalf("empty response: status=%d body=%s", response.Code, response.Body.String())
	}
}

type invalidController struct{}

func (*invalidController) Invalid(_ string) {}

func TestBindRejectsInvalidAndDuplicateRoutes(t *testing.T) {
	if err := ghttp.NewServer().Bind(&invalidController{}); err == nil || !strings.Contains(err.Error(), "no exported methods") {
		t.Fatalf("controller without route methods error = %v", err)
	}
	server := ghttp.NewServer()
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(&helloController{}); err == nil {
		t.Fatal("duplicate route was accepted")
	}
}

func TestRouteRegistrySnapshotAndOpenAPIPath(t *testing.T) {
	registry := router.NewRegistry()
	if err := registry.Register(router.RouteMetadata{Method: http.MethodGet, Path: "/users/:id"}); err != nil {
		t.Fatal(err)
	}
	routes := registry.List()
	routes[0].Path = "/changed"
	if registry.List()[0].Path != "/users/:id" {
		t.Fatal("route registry returned a mutable route snapshot")
	}
	server := ghttp.NewServer()
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.OpenAPI().Paths["/users/{id}"]; !ok {
		t.Fatal("OpenAPI path parameter was not converted to braces")
	}
}

func TestGroupBindAndMiddleware(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	called := false
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(func(context *gin.Context) { called = true; context.Next() })
		if err := group.Bind(&helloController{}); err != nil {
			t.Fatal(err)
		}
	})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/hello", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"message":"hello"`) || !called {
		t.Fatalf("group response: status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}
	if routes := server.Routes(); len(routes) != 1 || routes[0].Path != "/api/hello" {
		t.Fatalf("group routes = %#v", routes)
	}
}

type groupedPathRequest struct {
	g.Meta `path:"/items" method:"get"`
	Tenant string `path:"tenant"`
}

type groupedPathController struct{}

func (*groupedPathController) List(_ context.Context, request *groupedPathRequest) (*helloResponse, error) {
	return &helloResponse{Message: request.Tenant}, nil
}

func TestGroupBindValidatesPathBindingsAfterPrefix(t *testing.T) {
	server := ghttp.NewServer()
	server.Group("/tenants/:tenant", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&groupedPathController{}); err != nil {
			t.Fatalf("group.Bind() error = %v", err)
		}
	})

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tenants/acme/items", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"message":"acme"`) {
		t.Fatalf("group path binding: status=%d body=%s", response.Code, response.Body.String())
	}
	routes := server.Routes()
	if len(routes) != 1 || routes[0].Path != "/tenants/:tenant/items" {
		t.Fatalf("group path route = %#v", routes)
	}
}

func TestApplicationMiddlewareMustBeConfiguredBeforeBind(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	if err := server.Use(func(context *gin.Context) { context.Next() }); err == nil {
		t.Fatal("application middleware was accepted after routes were bound")
	}
}

type metaRequest struct {
	g.Meta `      path:"/users/:id" method:"get" tags:"User, Admin" summary:"Get user" description:"Get a user" operationId:"getUser" deprecated:"true" security:"bearer read" produces:"application/json" consumes:"application/json"`
	ID     int64 `path:"id"`
}

func TestParseMeta(t *testing.T) {
	metadata, err := router.ParseMeta(reflect.TypeOf((*metaRequest)(nil)))
	if err != nil || metadata.Method != http.MethodGet || metadata.Path != "/users/:id" || len(metadata.Tags) != 2 ||
		!metadata.Deprecated ||
		metadata.OperationID != "getUser" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	type requestWithoutMeta struct{}
	if _, err := router.ParseMeta(reflect.TypeOf((*requestWithoutMeta)(nil))); err == nil {
		t.Fatal("request without Meta was accepted")
	}
}

func TestRequestBindingSources(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/users/42?page=3", strings.NewReader(`{"name":"Lane","age":28}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	request.AddCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"id":42`, `"page":3`, `"token":"Bearer token"`, `"session":"session-1"`, `"name":"Lane"`, `"age":28`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("body %q does not contain %q", response.Body.String(), expected)
		}
	}
}

func TestRequestBindingValidationAndFormConversion(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/users/42?page=1", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "request validation failed") {
		t.Fatalf("validation response: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := server.Bind(&formController{}); err != nil {
		t.Fatal(err)
	}
	values := url.Values{"name": {"Lane"}, "count": {"4"}}
	request = httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"count":4`) {
		t.Fatalf("form response: status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/form", strings.NewReader("name=Lane&count=bad"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("conversion status=%d", response.Code)
	}
}

type defaultRequest struct {
	g.Meta `path:"/defaults" method:"get"`
	Page   int    `query:"page" default:"1" validate:"gte=1"`
	Sort   string `query:"sort" d:"createdAt"`
}

type defaultResponse struct {
	Page int    `json:"page"`
	Sort string `json:"sort"`
}

type defaultController struct{}

func (*defaultController) List(_ context.Context, request *defaultRequest) (*defaultResponse, error) {
	return &defaultResponse{Page: request.Page, Sort: request.Sort}, nil
}

func TestRequestParameterDefaultsAreAppliedBeforeValidation(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&defaultController{}); err != nil {
		t.Fatal(err)
	}
	operation := server.OpenAPI().Paths["/defaults"]["get"]
	foundShortDefault := false
	for _, parameter := range operation.Parameters {
		if parameter["name"] != "sort" {
			continue
		}
		foundShortDefault = true
		schema, ok := parameter["schema"].(map[string]any)
		if !ok || schema["default"] != "createdAt" {
			t.Fatalf("short default tag was not added to OpenAPI: %#v", parameter)
		}
	}
	if !foundShortDefault {
		t.Fatal("sort parameter is missing from OpenAPI")
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/defaults", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"page":1`) ||
		!strings.Contains(response.Body.String(), `"sort":"createdAt"`) {
		t.Fatalf("default parameters: status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/defaults?page=3&sort=name", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"page":3`) ||
		!strings.Contains(response.Body.String(), `"sort":"name"`) {
		t.Fatalf("explicit parameters: status=%d body=%s", response.Code, response.Body.String())
	}
}

type customEncoder struct{}

func (customEncoder) Encode(ctx *ghttp.Context, data any) error {
	ctx.JSON(http.StatusCreated, map[string]any{"custom": data != nil})
	return nil
}

func TestResponseEncoderCanBeReplaced(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithResponseEncoder(customEncoder{}))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"custom":true`) {
		t.Fatalf("custom response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestContextResponseHelpers(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&textController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/text", nil))
	if response.Code != http.StatusOK || response.Body.String() != "hello world" {
		t.Fatalf("text response: status=%d body=%q", response.Code, response.Body.String())
	}
}
