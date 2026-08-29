package router_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/router"
)

type independentSourceRequest struct {
	Path   string                `path:"value"`
	Query  string                `query:"value"`
	Header string                `header:"value"`
	Cookie string                `cookie:"value"`
	File   *multipart.FileHeader `file:"value"`
}

func TestBinderBindsPathQueryHeaderCookieAndFileIndependently(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("value", "upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("file contents")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest("POST", "/items/path?value=query", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("value", "header")
	request.AddCookie(&http.Cookie{Name: "value", Value: "cookie"})
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	context.Params = gin.Params{{Key: "value", Value: "path"}}

	binder, err := router.NewBinder(reflect.TypeOf((*independentSourceRequest)(nil)))
	if err != nil {
		t.Fatalf("NewBinder() error = %v", err)
	}
	var target independentSourceRequest
	if err := binder.Bind(context, &target, 32<<20); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if target.Path != "path" {
		t.Errorf("path = %q, want path", target.Path)
	}
	if target.Query != "query" {
		t.Errorf("query = %q, want query", target.Query)
	}
	if target.Header != "header" {
		t.Errorf("header = %q, want header", target.Header)
	}
	if target.Cookie != "cookie" {
		t.Errorf("cookie = %q, want cookie", target.Cookie)
	}
	if target.File == nil {
		t.Fatal("file was not bound")
	}
	if target.File.Filename != "upload.txt" {
		t.Errorf("file name = %q, want upload.txt", target.File.Filename)
	}
}
