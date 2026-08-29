package ghttp_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/openapi"
	"github.com/lanechi/gonex/router"
)

type wrappedErrorRequest struct {
	g.Meta `path:"/wrapped-error" method:"get"`
}

type wrappedErrorController struct{}

func (*wrappedErrorController) Read(context.Context, *wrappedErrorRequest) (*helloResponse, error) {
	return nil, fmt.Errorf("service layer: %w", ghttp.NewError(44004, http.StatusNotFound, "wrapped not found"))
}

type unsafeRequest struct {
	g.Meta `path:"/unsafe" method:"post"`
}

type unsafeController struct{}

func (*unsafeController) Write(context.Context, *unsafeRequest) (*helloResponse, error) {
	return &helloResponse{Message: "written"}, nil
}

func TestStrictJSONAndWrappedFrameworkError(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/users/42?page=1",
		strings.NewReader(`{"name":"Lane"} {"name":"second"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "invalid JSON request body") {
		t.Fatalf("trailing JSON response: status=%d body=%s", response.Code, response.Body.String())
	}

	if err := server.Bind(&wrappedErrorController{}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/wrapped-error", nil))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"code":44004`) ||
		strings.Contains(response.Body.String(), "service layer") {
		t.Fatalf("wrapped error response: status=%d body=%s", response.Code, response.Body.String())
	}
}

type missingPathRequest struct {
	g.Meta `path:"/missing/:id" method:"get"`
}

type missingPathController struct{}

func (*missingPathController) Read(context.Context, *missingPathRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

type extraPathRequest struct {
	g.Meta `       path:"/extra" method:"get"`
	ID     string `path:"id"`
}

type extraPathController struct{}

func (*extraPathController) Read(context.Context, *extraPathRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

type invalidPathRequest struct {
	g.Meta `path:"/invalid/user:id" method:"get"`
}

type invalidPathController struct{}

func (*invalidPathController) Read(context.Context, *invalidPathRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

type invalidFileRequest struct {
	g.Meta `       path:"/invalid-file" method:"post"`
	File   string `                                   file:"file"`
}

type invalidFileController struct{}

func (*invalidFileController) Upload(context.Context, *invalidFileRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

type invalidResponseController struct{}

func (*invalidResponseController) Read(context.Context, *helloRequest) (chan string, error) {
	return nil, nil
}

type reviewItem struct {
	ID int `json:"id"`
}

type appUserReviewsRes []reviewItem
type reviewMapRes map[string]int
type reviewCountRes int

type flexibleResponseRequest struct {
	g.Meta `path:"/flexible/list" method:"get"`
}

type flexibleResponsePointerRequest struct {
	g.Meta `path:"/flexible/list-pointer" method:"get"`
}

type flexibleResponseMapRequest struct {
	g.Meta `path:"/flexible/map" method:"get"`
}

type flexibleResponseMapPointerRequest struct {
	g.Meta `path:"/flexible/map-pointer" method:"get"`
}

type flexibleResponseScalarRequest struct {
	g.Meta `path:"/flexible/count" method:"get"`
}

type flexibleResponseController struct{}

func (*flexibleResponseController) List(context.Context, *flexibleResponseRequest) (appUserReviewsRes, error) {
	return appUserReviewsRes{{ID: 7}}, nil
}

func (*flexibleResponseController) ListPointer(context.Context, *flexibleResponsePointerRequest) (*appUserReviewsRes, error) {
	value := appUserReviewsRes{{ID: 8}}
	return &value, nil
}

func (*flexibleResponseController) Map(context.Context, *flexibleResponseMapRequest) (reviewMapRes, error) {
	return reviewMapRes{"ok": 1}, nil
}

func (*flexibleResponseController) MapPointer(context.Context, *flexibleResponseMapPointerRequest) (*reviewMapRes, error) {
	value := reviewMapRes{"ok": 2}
	return &value, nil
}

func (*flexibleResponseController) Count(context.Context, *flexibleResponseScalarRequest) (reviewCountRes, error) {
	return 3, nil
}

func TestControllerSupportsJSONResponseValuesAndPointers(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&flexibleResponseController{}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path     string
		contains string
	}{
		{"/flexible/list", `"id":7`},
		{"/flexible/list-pointer", `"id":8`},
		{"/flexible/map", `"ok":1`},
		{"/flexible/map-pointer", `"ok":2`},
		{"/flexible/count", `"data":3`},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Errorf("%s: status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}

	operation := server.OpenAPI().Paths["/flexible/list"]["get"]
	content := operation.Responses["200"].(map[string]any)["content"].(map[string]any)
	schema := content["application/json"].(map[string]any)["schema"].(map[string]any)
	data := schema["properties"].(map[string]any)["data"].(map[string]any)
	if data["type"] != "array" || data["items"].(map[string]any)["type"] != "object" {
		t.Fatalf("named slice response schema=%#v", data)
	}
	mapOperation := server.OpenAPI().Paths["/flexible/map"]["get"]
	mapContent := mapOperation.Responses["200"].(map[string]any)["content"].(map[string]any)
	mapSchema := mapContent["application/json"].(map[string]any)["schema"].(map[string]any)["properties"].(map[string]any)["data"].(map[string]any)
	if mapSchema["type"] != "object" || mapSchema["additionalProperties"].(map[string]any)["type"] != "integer" {
		t.Fatalf("named map response schema=%#v", mapSchema)
	}
}

type documentationConflictRequest struct {
	g.Meta `path:"/openapi.json" method:"get"`
}

type documentationConflictController struct{}

func (*documentationConflictController) Read(context.Context, *documentationConflictRequest) (*helloResponse, error) {
	return &helloResponse{}, nil
}

type RequiredEmbeddedQuery struct {
	Value string `query:"value"`
}

type requiredEmbeddedRequest struct {
	g.Meta                 `path:"/required-embedded" method:"get"`
	*RequiredEmbeddedQuery `binding:"required"`
}

type requiredEmbeddedController struct{}

func (*requiredEmbeddedController) Read(_ context.Context, request *requiredEmbeddedRequest) (*helloResponse, error) {
	return &helloResponse{Message: request.Value}, nil
}

func TestBinderDoesNotMaterializeMissingEmbeddedPointer(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&requiredEmbeddedController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/required-embedded", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing embedded pointer status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/required-embedded?value=ready", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"message":"ready"`) {
		t.Fatalf("bound embedded pointer status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBindRejectsInvalidRouteContractsWithoutPanicking(t *testing.T) {
	tests := []struct {
		controller any
		contains   string
	}{
		{&missingPathController{}, "no matching request field"},
		{&extraPathController{}, "is not declared"},
		{&invalidPathController{}, "wildcard must occupy"},
		{&invalidFileController{}, "unsupported type"},
		{&invalidResponseController{}, "JSON-encodable response type"},
		{&documentationConflictController{}, "conflicts with registered route"},
	}
	for _, test := range tests {
		server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
		err := server.Bind(test.controller)
		if err == nil || !strings.Contains(err.Error(), test.contains) {
			t.Errorf("Bind(%T) error=%v, want substring %q", test.controller, err, test.contains)
		}
	}
}

func TestRouteScopedMiddlewareAndRouteTable(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	called := false
	configuredCalled := false
	middleware := func(context *gin.Context) {
		called = true
		context.Next()
	}
	if err := server.Route(http.MethodGet, "/hello").Use(func(context *gin.Context) {
		configuredCalled = true
		context.Next()
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(&helloController{}, middleware); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Code != http.StatusOK || !called || !configuredCalled {
		t.Fatalf("route middleware: status=%d bind=%v configured=%v", response.Code, called, configuredCalled)
	}
	if err := server.Route(http.MethodGet, "/hello").Use(middleware); err == nil {
		t.Fatal("route middleware was accepted after Bind")
	}
	table := server.RouteTable()
	for _, expected := range []string{"helloController.Hello", "OpenAPI", "Swagger"} {
		if !strings.Contains(table, expected) {
			t.Errorf("route table does not contain %q:\n%s", expected, table)
		}
	}
}

func TestNilMiddlewareIsRejectedBeforeRouteRegistration(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Route(http.MethodGet, "/hello").Use(nil); err == nil || !strings.Contains(err.Error(), "middleware at index 0 is nil") {
		t.Fatalf("Route.Use(nil) error=%v", err)
	}
	if err := server.Bind(&helloController{}, nil); err == nil || !strings.Contains(err.Error(), "middleware at index 0 is nil") {
		t.Fatalf("Bind(..., nil) error=%v", err)
	}

	group := &ghttp.RouterGroup{}
	if group.Middleware(nil) != group {
		t.Fatal("RouterGroup.Middleware(nil) did not preserve the group")
	}
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(nil)
		if err := group.Bind(&helloController{}); err == nil || !strings.Contains(err.Error(), "middleware at index 0 is nil") {
			t.Fatalf("RouterGroup.Bind after Middleware(nil) error=%v", err)
		}
	})
}

type middlewareOrderRequest struct {
	g.Meta `path:"/order" method:"get"`
}

type middlewareOrderController struct {
	events *[]string
}

func (controller *middlewareOrderController) Read(context.Context, *middlewareOrderRequest) (*helloResponse, error) {
	*controller.events = append(*controller.events, "controller")
	return &helloResponse{Message: "ordered"}, nil
}

func TestApplicationGroupAndRouteMiddlewareOrder(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	events := make([]string, 0, 7)
	middleware := func(name string) ghttp.Middleware {
		return func(context *gin.Context) {
			events = append(events, name+"-before")
			context.Next()
			events = append(events, name+"-after")
		}
	}
	if err := server.Use(middleware("application")); err != nil {
		t.Fatal(err)
	}
	if err := server.Route(http.MethodGet, "/api/order").Use(middleware("route")); err != nil {
		t.Fatal(err)
	}
	server.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware("group"))
		if err := group.Bind(&middlewareOrderController{events: &events}); err != nil {
			t.Fatal(err)
		}
	})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/order", nil))
	want := "application-before,group-before,route-before,controller,route-after,group-after,application-after"
	if got := strings.Join(events, ","); response.Code != http.StatusOK || got != want {
		t.Fatalf("middleware order status=%d got=%q want=%q", response.Code, got, want)
	}
}

func TestDynamicSecurityMiddlewareConfiguration(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithAllowedHosts("old.example.com"))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	server.SetAllowedHosts("new.example.com")
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "old.example.com"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("old host status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "new.example.com"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("new host status=%d", response.Code)
	}

	if err := server.EnableCORS(ghttp.CORSOptions{Enabled: true, AllowOrigins: []string{"https://client.example.com"}}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "new.example.com"
	request.Header.Set("Origin", "https://client.example.com")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Header().Get("Access-Control-Allow-Origin") != "https://client.example.com" {
		t.Fatalf("dynamic CORS origin=%q", response.Header().Get("Access-Control-Allow-Origin"))
	}
	if err := server.EnableCORS(ghttp.CORSOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := server.Bind(&unsafeController{}); err != nil {
		t.Fatal(err)
	}
	server.EnableCSRF(ghttp.CSRFOptions{Enabled: true})
	bootstrap := httptest.NewRecorder()
	bootstrapRequest := httptest.NewRequest(http.MethodGet, "/hello", nil)
	bootstrapRequest.Host = "new.example.com"
	server.ServeHTTP(bootstrap, bootstrapRequest)
	csrfCookies := bootstrap.Result().Cookies()
	if len(csrfCookies) == 0 {
		t.Fatal("CSRF bootstrap cookie is missing")
	}
	request = httptest.NewRequest(http.MethodPost, "/unsafe", nil)
	request.Host = "new.example.com"
	request.AddCookie(csrfCookies[0])
	request.Header.Set("X-CSRF-Token", csrfCookies[0].Value)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("valid CSRF status=%d body=%s", response.Code, response.Body.String())
	}
	server.EnableCSRF(ghttp.CSRFOptions{})
	request = httptest.NewRequest(http.MethodPost, "/unsafe", nil)
	request.Host = "new.example.com"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("disabled CSRF status=%d", response.Code)
	}
}

func TestInitializationErrorsAreObservable(t *testing.T) {
	server := ghttp.NewServer(
		ghttp.WithLogger(&recordingLogger{}),
		ghttp.WithCORS(ghttp.CORSOptions{Enabled: true, AllowCredentials: true}),
	)
	if server.Err() == nil || !strings.Contains(server.Err().Error(), "allowed origin") {
		t.Fatalf("invalid CORS initialization error=%v", server.Err())
	}
	if err := server.Bind(&helloController{}); err == nil {
		t.Fatal("Bind accepted a server with invalid initialization")
	}

	missingRoot := filepath.Join(t.TempDir(), "missing")
	server = ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithTemplateRoot(missingRoot))
	if server.Err() == nil || !strings.Contains(server.Err().Error(), "template root") {
		t.Fatalf("invalid template initialization error=%v", server.Err())
	}

	logger := &recordingLogger{}
	server = ghttp.NewServer(ghttp.WithLogger(logger))
	server.HTTPServer().ErrorLog.Print("connection failure")
	found := false
	for index, message := range logger.Messages() {
		if message != "connection failure" {
			continue
		}
		fields := make(map[string]any)
		for _, field := range logger.FieldEntries(index) {
			fields[field.Key] = field.Value
		}
		if fields["logger"] == "net/http" {
			found = true
		}
	}
	if !found {
		t.Fatal("net/http ErrorLog was not routed through the framework logger")
	}
}

func TestStaticRootIndexDirectoriesAndValidation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("root-index"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "index.html"), []byte("docs-index"), 0600); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	if err := server.Static("/site", root); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]string{"/site/": "root-index", "/site/docs/": "docs-index"} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != expected {
			t.Errorf("%s status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
	if err := server.Static("invalid", root); err == nil {
		t.Fatal("invalid static URI was accepted")
	}
	if err := server.StaticFS("/nil", nil); err == nil {
		t.Fatal("nil static filesystem was accepted")
	}
	if err := server.StaticFS("/embedded", fstest.MapFS{"index.html": {Data: []byte("embedded")}}); err != nil {
		t.Fatal(err)
	}
}

type optionalBodyRequest struct {
	g.Meta `       path:"/optional" method:"post"`
	Name   string `                               json:"name"`
	Count  int    `                                           query:"count" validate:"gt=1,lt=5"`
}

type embeddedData struct {
	Value string `json:"value"`
}

type embeddedResponse struct {
	embeddedData
}

type optionalBodyController struct{}

func (*optionalBodyController) Write(context.Context, *optionalBodyRequest) (*embeddedResponse, error) {
	return &embeddedResponse{}, nil
}

func TestOpenAPIConstraintsEmbeddedSchemaAndSwaggerEscaping(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&optionalBodyController{}); err != nil {
		t.Fatal(err)
	}
	operation := server.OpenAPI().Paths["/optional"]["post"]
	if _, required := operation.RequestBody["required"]; required {
		t.Fatalf("optional request body was marked required: %#v", operation.RequestBody)
	}
	countSchema := operation.Parameters[0]["schema"].(map[string]any)
	if countSchema["minimum"] != float64(1) || countSchema["exclusiveMinimum"] != true ||
		countSchema["maximum"] != float64(5) ||
		countSchema["exclusiveMaximum"] != true {
		t.Fatalf("exclusive numeric constraints=%#v", countSchema)
	}
	response := operation.Responses["200"].(map[string]any)
	content := response["content"].(map[string]any)["application/json"].(map[string]any)
	envelope := content["schema"].(map[string]any)
	data := envelope["properties"].(map[string]any)["data"].(map[string]any)
	if _, ok := data["properties"].(map[string]any)["value"]; !ok {
		t.Fatalf("embedded response schema=%#v", data)
	}

	html := string(openapi.RenderSwaggerHTML(`/spec" onmouseover="alert(1)`))
	if strings.Contains(html, `spec-url="/spec" onmouseover=`) || !strings.Contains(html, "&#34;") {
		t.Fatalf("Swagger spec URL was not HTML escaped: %s", html)
	}
}

func TestRouteRegistryDeepSnapshot(t *testing.T) {
	registry := router.NewRegistry()
	bindings := []router.FieldBinding{{Index: []int{1}, Query: "page"}}
	if err := registry.Register(router.RouteMetadata{Method: http.MethodGet, Path: "/snapshot", Bindings: bindings}); err != nil {
		t.Fatal(err)
	}
	bindings[0].Index[0] = 7
	snapshot := registry.List()
	snapshot[0].Bindings[0].Index[0] = 99
	if got := registry.List()[0].Bindings[0].Index[0]; got != 1 {
		t.Fatalf("registry binding index leaked through snapshot: %d", got)
	}
}

func TestMultipleServersKeepRoutesOpenAPIAndRequestLogsIndependent(t *testing.T) {
	apiLogger := &recordingLogger{}
	adminLogger := &recordingLogger{}
	api := ghttp.NewServer(ghttp.WithName("api"), ghttp.WithLogger(apiLogger))
	admin := ghttp.NewServer(ghttp.WithName("admin"), ghttp.WithLogger(adminLogger))
	if err := api.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	admin.Group("/admin", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&helloController{}); err != nil {
			t.Fatal(err)
		}
	})
	apiLogger.messages, apiLogger.fields = nil, nil
	adminLogger.messages, adminLogger.fields = nil, nil
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Code != http.StatusOK || api.OpenAPI().Info.Title != "api" || admin.OpenAPI().Info.Title != "admin" ||
		len(api.Routes()) != 1 ||
		api.Routes()[0].Path != "/hello" ||
		len(admin.Routes()) != 1 ||
		admin.Routes()[0].Path != "/admin/hello" {
		t.Fatalf("multi-server state: apiRoutes=%#v adminRoutes=%#v", api.Routes(), admin.Routes())
	}
	if len(apiLogger.Messages()) == 0 || len(adminLogger.Messages()) != 0 {
		t.Fatalf("request logs crossed servers: api=%v admin=%v", apiLogger.Messages(), adminLogger.Messages())
	}
}

func TestRunContextLifecycleAndTaskShutdown(t *testing.T) {
	logger := &recordingLogger{}
	server := ghttp.NewServer(ghttp.WithLogger(logger), ghttp.WithAddress("127.0.0.1:0"))
	events := make([]string, 0, 4)
	started := make(chan struct{})
	taskStopped := make(chan struct{})
	server.OnStart(func(context.Context) error { events = append(events, "start"); return nil })
	server.OnStarted(func(context.Context) error { events = append(events, "started"); close(started); return nil })
	server.OnShutdown(func(context.Context) error { events = append(events, "shutdown"); return nil })
	server.OnStop(func(context.Context) error { events = append(events, "stop"); return nil })
	server.Go(func(ctx context.Context) { <-ctx.Done(); close(taskStopped) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.RunContext(ctx) }()
	select {
	case <-started:
	case err := <-done:
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skipf("sandbox does not permit a loopback listener: %v", err)
		}
		t.Fatalf("server exited before OnStarted: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not reach OnStarted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
	select {
	case <-taskStopped:
	default:
		t.Fatal("tracked task was not canceled")
	}
	if strings.Join(events, ",") != "start,started,shutdown,stop" {
		t.Fatalf("lifecycle events=%v", events)
	}
	messages := strings.Join(logger.Messages(), "\n")
	if !strings.Contains(messages, "收到退出信号，开始优雅退出") || !strings.Contains(messages, "服务已优雅退出") {
		t.Fatalf("graceful shutdown messages=%q", messages)
	}
	server.Go(func(context.Context) { t.Error("task added after shutdown was started") })
}

type typedBindingRequest struct {
	g.Meta  `path:"/typed" method:"get"`
	When    time.Time     `                           query:"when"`
	Timeout time.Duration `                           query:"timeout"`
}

type typedBindingResponse struct {
	When    string `json:"when"`
	Timeout string `json:"timeout"`
}

type typedBindingController struct{}

func (*typedBindingController) Read(_ context.Context, request *typedBindingRequest) (*typedBindingResponse, error) {
	return &typedBindingResponse{When: request.When.UTC().Format(time.RFC3339), Timeout: request.Timeout.String()}, nil
}

func TestBinderSupportsTextAndDuration(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&typedBindingController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/typed?when=2026-08-20T12:30:00Z&timeout=2s", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"when":"2026-08-20T12:30:00Z"`) ||
		!strings.Contains(response.Body.String(), `"timeout":"2s"`) {
		t.Fatalf("typed binding response: status=%d body=%s", response.Code, response.Body.String())
	}
}
