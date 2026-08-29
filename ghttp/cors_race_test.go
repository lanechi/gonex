package ghttp

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEnableCORSOwnsConfigurationSlices(t *testing.T) {
	server := NewServer()
	server.engine.GET("/cors", func(context *gin.Context) { context.Status(http.StatusNoContent) })
	options := CORSOptions{
		Enabled:      true,
		AllowOrigins: []string{"https://allowed.example"},
		AllowMethods: []string{http.MethodGet},
	}
	if err := server.EnableCORS(options); err != nil {
		t.Fatal(err)
	}

	var mutations sync.WaitGroup
	mutations.Add(1)
	go func() {
		defer mutations.Done()
		for index := 0; index < 2000; index++ {
			if index%2 == 0 {
				options.AllowOrigins[0] = "https://mutated.example"
			} else {
				options.AllowOrigins[0] = "https://other.example"
			}
		}
	}()
	for index := 0; index < 200; index++ {
		request := httptest.NewRequest(http.MethodGet, "/cors", nil)
		request.Header.Set("Origin", "https://allowed.example")
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
			t.Fatalf("allow origin = %q", got)
		}
	}
	mutations.Wait()
}

func TestEnableCORSRejectsInvalidUpdateWithoutChangingActivePolicy(t *testing.T) {
	server := NewServer()
	server.engine.GET("/cors", func(context *gin.Context) { context.Status(http.StatusNoContent) })
	if err := server.EnableCORS(CORSOptions{Enabled: true, AllowOrigins: []string{"https://allowed.example"}}); err != nil {
		t.Fatal(err)
	}
	if err := server.EnableCORS(CORSOptions{Enabled: true, AllowOrigins: []string{"*"}, AllowCredentials: true}); err == nil {
		t.Fatal("invalid CORS update was accepted")
	}

	request := httptest.NewRequest(http.MethodGet, "/cors", nil)
	request.Header.Set("Origin", "https://allowed.example")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
		t.Fatalf("active CORS policy changed after rejected update: %q", got)
	}
}
