package ghttp

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/ghttp/internal/ginruntime"
)

// Gin mode values supported by the framework.
const (
	DebugMode   = gin.DebugMode
	ReleaseMode = gin.ReleaseMode
	TestMode    = gin.TestMode
)

// WithMode configures Gin's process-wide mode before Server initialization.
// The option takes precedence over server.mode in configuration.
func WithMode(mode string) Option {
	return func(server *Server) {
		server.mode = strings.TrimSpace(mode)
		server.options.Mode = optional[string]{Value: mode, Set: true}
	}
}

// Mode returns the Gin mode used by this Server.
func (server *Server) Mode() string {
	if server == nil || server.mode == "" {
		return ReleaseMode
	}
	return server.mode
}

// IsDebug reports whether this Server runs in debug mode.
//
// The value belongs to the Server instance and does not read Gin's global
// mode, so components can make decisions for the correct server when more
// than one server is used in a process.
func (server *Server) IsDebug() bool {
	return server != nil && server.debug
}

func (server *Server) applyModeConfig() {
	mode := server.mode
	if !server.options.Mode.Set && server.config != nil {
		mode = configString(server.config.Get("server.mode"))
	}
	normalized, err := normalizeGinMode(mode)
	if err != nil {
		server.addInitializationError(err)
		server.mode = ReleaseMode
		server.debug = false
		return
	}
	server.mode = normalized
	server.debug = normalized == DebugMode
}

func normalizeGinMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ReleaseMode, nil
	}
	switch mode {
	case DebugMode, ReleaseMode, TestMode:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid gin mode %q: available modes are %s, %s, %s", mode, DebugMode, ReleaseMode, TestMode)
	}
}

func setGinMode(mode string) {
	ginruntime.SetMode(mode)
}
