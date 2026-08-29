package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/template-demo/internal/controller"
	"github.com/lanechi/gonex/ghttp"
)

func TestTemplatePage(t *testing.T) {
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(
		ghttp.WithConfig(config.Default()),
		ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}),
	)
	defer func() { _ = server.Close() }()
	if err := server.AddTemplateFunc("upper", strings.ToUpper); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(controller.NewPage()); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/page?name=Lane", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	body, err := io.ReadAll(response.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !strings.Contains(string(body), "Hello, Lane!") {
		t.Fatalf("unexpected template response: status=%d body=%s", response.Code, body)
	}
}
