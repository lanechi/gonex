package benchmark_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	frameworkg "github.com/lanechi/gonex/g"
	frameworkhttp "github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

type helloRequest struct {
	frameworkg.Meta `path:"/hello" method:"get"`
}

type helloResponse struct {
	Message string `json:"message"`
}

type helloController struct{}

func (*helloController) Hello(_ context.Context, _ *helloRequest) (*helloResponse, error) {
	return &helloResponse{Message: "hello"}, nil
}

type bindingRequest struct {
	frameworkg.Meta `path:"/users/:id" method:"post"`
	ID              int64  `path:"id" binding:"required"`
	Page            int    `query:"page" binding:"gte=1"`
	Token           string `header:"Authorization" binding:"required"`
	Session         string `cookie:"session_id"`
	Name            string `json:"name" binding:"required"`
	Age             int    `json:"age" validate:"gte=0"`
}

type bindingResponse struct {
	ID      int64  `json:"id"`
	Page    int    `json:"page"`
	Token   string `json:"token"`
	Session string `json:"session"`
	Name    string `json:"name"`
	Age     int    `json:"age"`
}

type bindingController struct{}

func (*bindingController) Create(_ context.Context, request *bindingRequest) (*bindingResponse, error) {
	return &bindingResponse{
		ID: request.ID, Page: request.Page, Token: request.Token, Session: request.Session,
		Name: request.Name, Age: request.Age,
	}, nil
}

var benchmarkResponse = frameworkhttp.Response{
	Code: 0, Message: "OK", Data: helloResponse{Message: "hello"},
}

func newFrameworkServer(withRequestID bool) *frameworkhttp.Server {
	return frameworkhttp.NewServer(
		frameworkhttp.WithLogger(logging.NewNopLogger()),
		frameworkhttp.WithRequestID(withRequestID),
		frameworkhttp.WithOpenAPI(false),
	)
}

func BenchmarkFrameworkController(b *testing.B) {
	server := newFrameworkServer(true)
	if err := server.Bind(&helloController{}); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	benchmarkHTTP(b, server, request)
}

func BenchmarkFrameworkControllerNoRequestID(b *testing.B) {
	server := newFrameworkServer(false)
	if err := server.Bind(&helloController{}); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	benchmarkHTTP(b, server, request)
}

func BenchmarkGinNativeHandler(b *testing.B) {
	engine := gin.New()
	engine.GET("/hello", func(context *gin.Context) {
		context.JSON(http.StatusOK, benchmarkResponse)
	})
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	benchmarkHTTP(b, engine, request)
}

func BenchmarkFrameworkJSONBinding(b *testing.B) {
	server := newFrameworkServer(false)
	if err := server.Bind(&bindingController{}); err != nil {
		b.Fatal(err)
	}
	benchmarkBindingHTTP(b, server)
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
		context.JSON(http.StatusOK, frameworkhttp.Response{
			Code: 0, Message: "OK", Data: bindingResponse{
				ID: id, Page: page, Token: token, Session: session,
				Name: payload.Name, Age: payload.Age,
			},
		})
	})
	benchmarkBindingHTTP(b, engine)
}

func benchmarkHTTP(b *testing.B, handler http.Handler, request *http.Request) {
	b.Helper()
	probeRequest := request.Clone(request.Context())
	probeRequest.Body = http.NoBody
	probeResponse := httptest.NewRecorder()
	handler.ServeHTTP(probeResponse, probeRequest)
	if probeResponse.Code != http.StatusOK || !bytes.Contains(probeResponse.Body.Bytes(), []byte(`"message":"hello"`)) {
		b.Fatalf("simple response probe failed: status=%d body=%s", probeResponse.Code, probeResponse.Body.String())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		iterationRequest := request.Clone(request.Context())
		iterationRequest.Body = http.NoBody
		handler.ServeHTTP(response, iterationRequest)
	}
}

func benchmarkBindingHTTP(b *testing.B, handler http.Handler) {
	b.Helper()
	payload := []byte(`{"name":"Lane"}`)
	probeRequest := httptest.NewRequest(http.MethodPost, "/users/42?page=1", bytes.NewReader(payload))
	probeRequest.Header.Set("Content-Type", "application/json")
	probeRequest.Header.Set("Authorization", "Bearer token")
	probeRequest.AddCookie(&http.Cookie{Name: "session_id", Value: "session"})
	probeResponse := httptest.NewRecorder()
	handler.ServeHTTP(probeResponse, probeRequest)
	if probeResponse.Code != http.StatusOK || !bytes.Contains(probeResponse.Body.Bytes(), []byte(`"id":42`)) || !bytes.Contains(probeResponse.Body.Bytes(), []byte(`"name":"Lane"`)) {
		b.Fatalf("binding response probe failed: status=%d body=%s", probeResponse.Code, probeResponse.Body.String())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		request := httptest.NewRequest(http.MethodPost, "/users/42?page=1", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer token")
		request.AddCookie(&http.Cookie{Name: "session_id", Value: "session"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
	}
}
