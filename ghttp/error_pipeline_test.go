package ghttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
)

func customMiddlewareErrorHandler(ctx *Context, err error) {
	status := http.StatusInternalServerError
	code := 50000
	var frameworkError *Error
	if errors.As(err, &frameworkError) {
		status = frameworkError.HTTPStatus
		code = frameworkError.Code
	}
	ctx.JSON(status, map[string]any{"custom": true, "code": code})
}

func TestServerMiddlewareFailuresUseConfiguredErrorHandler(t *testing.T) {
	t.Run("body limit", func(t *testing.T) {
		server := NewServer(
			WithLogger(logging.NewNopLogger()),
			WithRequestLimits(1, 0, 0),
			WithErrorHandler(customMiddlewareErrorHandler),
		)
		server.Engine().POST("/body", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
		request := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader("xx"))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertCustomMiddlewareFailure(t, response, http.StatusRequestEntityTooLarge, 41300)
	})

	t.Run("host validation", func(t *testing.T) {
		server := NewServer(
			WithLogger(logging.NewNopLogger()),
			WithAllowedHosts("api.example.com"),
			WithErrorHandler(customMiddlewareErrorHandler),
		)
		server.Engine().GET("/host", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
		request := httptest.NewRequest(http.MethodGet, "/host", nil)
		request.Host = "evil.example.com"
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertCustomMiddlewareFailure(t, response, http.StatusMisdirectedRequest, 42100)
	})

	t.Run("csrf", func(t *testing.T) {
		server := NewServer(
			WithLogger(logging.NewNopLogger()),
			WithCSRF(CSRFOptions{Enabled: true}),
			WithErrorHandler(customMiddlewareErrorHandler),
		)
		server.Engine().POST("/csrf", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
		request := httptest.NewRequest(http.MethodPost, "/csrf", nil)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		assertCustomMiddlewareFailure(t, response, http.StatusForbidden, 40300)
	})
}

func assertCustomMiddlewareFailure(t *testing.T, response *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"custom":true`) && !strings.Contains(body, `"custom": true`) {
		t.Fatalf("custom error handler was not used: %s", body)
	}
	if !strings.Contains(body, `"code":`+strconv.Itoa(code)) && !strings.Contains(body, `"code": `+strconv.Itoa(code)) {
		t.Fatalf("custom error code was not used: %s", body)
	}
}
