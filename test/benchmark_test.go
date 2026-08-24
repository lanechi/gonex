package ghttp_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

func benchmarkServer() *ghttp.Server {
	return ghttp.NewServer(ghttp.WithLogger(logging.NewNopLogger()))
}

func benchmarkServerWithoutRequestID() *ghttp.Server {
	return ghttp.NewServer(ghttp.WithLogger(logging.NewNopLogger()), ghttp.WithRequestID(false))
}

func BenchmarkFrameworkMetaHandler(b *testing.B) {
	server := benchmarkServer()
	if err := server.Bind(&helloController{}); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
	}
}

func BenchmarkGinNativeHandler(b *testing.B) {
	engine := gin.New()
	engine.GET("/hello", func(context *gin.Context) {
		context.JSON(http.StatusOK, ghttp.Response{
			Code: 0, Message: "OK", Data: helloResponse{Message: "hello"},
		})
	})
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
	}
}

func BenchmarkGinNativeJSONBinding(b *testing.B) {
	engine := gin.New()
	engine.POST("/users/:id", func(context *gin.Context) {
		var payload struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		if err := context.ShouldBindJSON(&payload); err != nil {
			context.Status(http.StatusBadRequest)
			return
		}
		id, _ := strconv.ParseInt(context.Param("id"), 10, 64)
		page, _ := strconv.Atoi(context.Query("page"))
		token := context.GetHeader("Authorization")
		session, _ := context.Cookie("session_id")
		context.JSON(http.StatusOK, ghttp.Response{
			Code: 0, Message: "OK", Data: bindingResponse{
				ID: id, Page: page, Token: token, Session: session,
				Name: payload.Name, Age: payload.Age,
			},
		})
	})
	payload := []byte(`{"name":"Lane"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		request := httptest.NewRequest(http.MethodPost, "/users/42", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer token")
		request.URL.RawQuery = "page=1"
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
	}
}

func BenchmarkFrameworkJSONBinding(b *testing.B) {
	server := benchmarkServer()
	if err := server.Bind(&bindingController{}); err != nil {
		b.Fatal(err)
	}
	payload := []byte(`{"name":"Lane"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		request := httptest.NewRequest(http.MethodPost, "/users/42", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer token")
		request.URL.RawQuery = "page=1"
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
	}
}

func BenchmarkFrameworkResponseEncoding(b *testing.B) {
	server := benchmarkServer()
	if err := server.Bind(&helloController{}); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
	}
}

func BenchmarkFrameworkResponseEncodingNoRequestID(b *testing.B) {
	server := benchmarkServerWithoutRequestID()
	if err := server.Bind(&helloController{}); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
	}
}
