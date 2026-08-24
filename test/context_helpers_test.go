package ghttp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type contextInspectRequest struct {
	g.Meta `       path:"/context/:id" method:"get"`
	ID     string `path:"id"`
	Query  string `                                 query:"query"`
	Header string `                                               header:"X-Test"`
}

type contextInspectResponse struct {
	ID        string `json:"id"`
	Query     string `json:"query"`
	Header    string `json:"header"`
	RequestID string `json:"request_id"`
	Ready     bool   `json:"ready"`
}

type redirectRequest struct {
	g.Meta `path:"/redirect" method:"get"`
}

type streamRequest struct {
	g.Meta `path:"/stream" method:"get"`
}

type fileRequest struct {
	g.Meta `path:"/context-file" method:"get"`
}

type missingFileRequest struct {
	g.Meta `path:"/missing-context-file" method:"get"`
}

type contextHelpersController struct {
	filePath string
}

func (*contextHelpersController) Inspect(
	ctx context.Context,
	request *contextInspectRequest,
) (*contextInspectResponse, error) {
	frameworkContext := ghttp.FromContext(ctx)
	frameworkContext.Set("ready", true)
	ready, exists := frameworkContext.Get("ready")
	if frameworkContext.Gin() == nil || frameworkContext.Request() == nil || frameworkContext.Response() == nil ||
		frameworkContext.Logger() == nil ||
		frameworkContext.ClientIP() == "" ||
		!exists {
		return nil, ghttp.NewError(50010, http.StatusInternalServerError, "context helpers are incomplete")
	}
	return &contextInspectResponse{
		ID: frameworkContext.Param(
			"id",
		), Query: frameworkContext.Query("query"), Header: frameworkContext.Header("X-Test"),
		RequestID: frameworkContext.RequestID(), Ready: ready.(bool) && request.ID == frameworkContext.Param("id"),
	}, nil
}

func (*contextHelpersController) Redirect(ctx context.Context, _ *redirectRequest) error {
	ghttp.FromContext(ctx).Redirect(http.StatusFound, "/context/redirected")
	return nil
}

func (*contextHelpersController) Stream(ctx context.Context, _ *streamRequest) error {
	ghttp.FromContext(ctx).Stream(http.StatusAccepted, "text/plain", func(writer io.Writer) bool {
		_, _ = io.WriteString(writer, "streamed")
		return false
	})
	return nil
}

func (controller *contextHelpersController) File(ctx context.Context, _ *fileRequest) error {
	ghttp.FromContext(ctx).File(http.StatusOK, controller.filePath)
	return nil
}

func (*contextHelpersController) MissingFile(ctx context.Context, _ *missingFileRequest) error {
	ghttp.FromContext(ctx).File(http.StatusOK, filepath.Join(os.TempDir(), "gonex-missing-file"))
	return nil
}

func TestContextFacadeAndResponseHelpers(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "download.txt")
	if err := os.WriteFile(filePath, []byte("download"), 0600); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.Bind(&contextHelpersController{filePath: filePath}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/context/42?query=value", nil)
	request.Header.Set("X-Test", "header-value")
	request.Header.Set("X-Request-ID", strings.Repeat("x", 200))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	requestID := response.Header().Get("X-Request-ID")
	if response.Code != http.StatusOK || len(requestID) != 32 ||
		!strings.Contains(response.Body.String(), `"id":"42"`) ||
		!strings.Contains(response.Body.String(), `"query":"value"`) ||
		!strings.Contains(response.Body.String(), `"header":"header-value"`) ||
		!strings.Contains(response.Body.String(), `"ready":true`) {
		t.Fatalf("context response: status=%d requestID=%q body=%s", response.Code, requestID, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/redirect", nil))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/context/redirected" {
		t.Fatalf("redirect response: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if response.Code != http.StatusAccepted || response.Body.String() != "streamed" ||
		!strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf(
			"stream response: status=%d contentType=%q body=%q",
			response.Code,
			response.Header().Get("Content-Type"),
			response.Body.String(),
		)
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/context-file", nil))
	if response.Code != http.StatusOK || response.Body.String() != "download" {
		t.Fatalf("file response: status=%d body=%q", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing-context-file", nil))
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("missing file response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestTemplateErrorsDoNotLeakPartialHTML(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("partial {{fail}} trailing"), 0600); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}))
	if err := server.AddTemplateFunc("fail", func() (string, error) { return "", fmt.Errorf("template failed") }); err != nil {
		t.Fatal(err)
	}
	if err := server.SetTemplateRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(&pageController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/page", nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "partial") ||
		!strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("template failure response: status=%d body=%q", response.Code, response.Body.String())
	}
}
