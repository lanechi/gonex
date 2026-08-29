package ghttp

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWithEnginePreservesTrustedProxyConfiguration(t *testing.T) {
	engine := gin.New()
	if err := engine.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(WithEngine(engine))
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	// Gin does not expose its trusted-proxy list directly. This test guards the
	// contract indirectly by asserting construction does not replace the engine;
	// package-level initialization tests cover the framework-owned default engine.
	if server.Engine() != engine {
		t.Fatal("WithEngine did not retain the supplied engine")
	}
}
