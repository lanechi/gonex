package ghttp_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

func TestServerModeLoadsFromConfiguration(t *testing.T) {
	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })

	configuration := config.New()
	configuration.Set("server.mode", ghttp.ReleaseMode)
	server := ghttp.NewServer(
		ghttp.WithConfig(configuration),
		ghttp.WithLogger(logging.NewNopLogger()),
		ghttp.WithOpenAPI(false),
	)
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	if server.Mode() != ghttp.ReleaseMode || server.IsDebug() || gin.Mode() != ghttp.ReleaseMode {
		t.Fatalf("server mode=%q, debug=%v, gin mode=%q, want release", server.Mode(), server.IsDebug(), gin.Mode())
	}
}

func TestServerModeOptionOverridesConfiguration(t *testing.T) {
	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })

	configuration := config.New()
	configuration.Set("server.mode", ghttp.ReleaseMode)
	server := ghttp.NewServer(
		ghttp.WithConfig(configuration),
		ghttp.WithMode(ghttp.DebugMode),
		ghttp.WithLogger(logging.NewNopLogger()),
		ghttp.WithOpenAPI(false),
	)
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	if server.Mode() != ghttp.DebugMode || !server.IsDebug() || gin.Mode() != ghttp.DebugMode {
		t.Fatalf("server mode=%q, debug=%v, gin mode=%q, want debug", server.Mode(), server.IsDebug(), gin.Mode())
	}
}

func TestServerRejectsInvalidConfiguredMode(t *testing.T) {
	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })

	configuration := config.New()
	configuration.Set("server.mode", "invalid")
	server := ghttp.NewServer(
		ghttp.WithConfig(configuration),
		ghttp.WithLogger(logging.NewNopLogger()),
		ghttp.WithOpenAPI(false),
	)
	if server.Err() == nil {
		t.Fatal("invalid configured Gin mode did not produce an initialization error")
	}
}
