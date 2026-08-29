package ghttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/openapi"
	"github.com/lanechi/gonex/router"
)

func TestOpenAPIAndSwaggerEndpoints(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithName("users"))
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("OpenAPI status=%d", response.Code)
	}
	for _, expected := range []string{`"openapi":"3.0.3"`, `"title":"users"`, `"/users/{id}"`, `"post"`, `"name"`, `"application/json"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("OpenAPI does not contain %q", expected)
		}
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="openapi-ui-container"`) || !strings.Contains(response.Body.String(), `spec-url="/openapi.json"`) {
		t.Fatalf("Swagger response: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "https://cdn.jsdelivr.net/npm/openapi-ui-dist@latest/lib/openapi-ui.umd.js") {
		t.Fatal("Swagger page must load the configured CDN UI script")
	}
	if response.Header().Get("Content-Security-Policy") != "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'unsafe-inline'; connect-src 'self'" {
		t.Fatalf("unexpected Swagger CSP: %q", response.Header().Get("Content-Security-Policy"))
	}
}

func TestOpenAPIUsesMetadataBindingContract(t *testing.T) {
	routes, err := router.ScanController(&uploadController{})
	if err != nil {
		t.Fatal(err)
	}
	routeMetadata := routes[0].Metadata
	operation := openapi.Generate("metadata", []router.RouteMetadata{routeMetadata}).Paths["/upload"]["post"]
	content := operation.RequestBody["content"].(map[string]any)
	if _, ok := content["multipart/form-data"]; !ok {
		t.Fatalf("OpenAPI used runtime binder instead of metadata: %#v", content)
	}
}

func TestOpenAPIMetadataAndCacheIsolation(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&documentedController{}); err != nil {
		t.Fatal(err)
	}
	document := server.OpenAPI()
	operation := document.Paths["/documented"]["post"]
	if operation.OperationID != "createUser" || !operation.Deprecated || len(operation.Security) != 1 {
		t.Fatalf("operation metadata=%#v", operation)
	}
	body := operation.RequestBody["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	name := body["properties"].(map[string]any)["name"].(map[string]any)
	if name["type"] != "string" || name["description"] != "Display name" || name["example"] != "Lane" || name["minLength"] != float64(3) {
		t.Fatalf("field schema=%#v", name)
	}
	if _, ok := document.Components["securitySchemes"].(map[string]any)["BearerAuth"]; !ok {
		t.Fatalf("security scheme missing")
	}
	document.Paths["/documented"]["post"] = openapi.Operation{}
	if server.OpenAPI().Paths["/documented"]["post"].OperationID != "createUser" {
		t.Fatal("OpenAPI cache leaked mutable map")
	}
}

func TestOpenAPIInfersMultipartForFileUpload(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&uploadController{}); err != nil {
		t.Fatal(err)
	}
	operation := server.OpenAPI().Paths["/upload"]["post"]
	content := operation.RequestBody["content"].(map[string]any)
	if _, ok := content["multipart/form-data"]; !ok {
		t.Fatalf("multipart content missing: %#v", content)
	}
	if _, ok := content["application/x-www-form-urlencoded"]; ok {
		t.Fatalf("file upload incorrectly exposed as URL-encoded form: %#v", content)
	}
	multipartSchema := content["multipart/form-data"].(map[string]any)["schema"].(map[string]any)
	fileSchema := multipartSchema["properties"].(map[string]any)["file"].(map[string]any)
	if fileSchema["type"] != "string" || fileSchema["format"] != "binary" {
		t.Fatalf("file schema=%#v", fileSchema)
	}
}

func TestOpenAPIAndSwaggerEndpointsShareOneSwitch(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	for _, path := range []string{"/openapi.json", "/docs/"} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status=%d", path, response.Code)
		}
	}
	server.EnableOpenAPI(true)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("enabled OpenAPI status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("enabled Swagger status=%d", response.Code)
	}
}
