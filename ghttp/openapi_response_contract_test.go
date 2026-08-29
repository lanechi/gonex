package ghttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type customEnvelopeEncoder struct{}

func (customEnvelopeEncoder) Encode(ctx *Context, data any) error {
	ctx.JSON(http.StatusOK, map[string]any{"success": true, "result": data})
	return nil
}

func TestCustomResponseEncoderDisablesBuiltInOpenAPIByDefault(t *testing.T) {
	server := NewServer(WithResponseEncoder(customEnvelopeEncoder{}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("custom response encoder left default OpenAPI enabled: status=%d", response.Code)
	}
}

func TestCustomResponseEncoderCanExplicitlyReenableOpenAPI(t *testing.T) {
	server := NewServer(
		WithResponseEncoder(customEnvelopeEncoder{}),
		WithOpenAPI(OpenAPIOptions{Enabled: true}),
	)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("explicit OpenAPI enable did not override response-encoder default: status=%d", response.Code)
	}
}
