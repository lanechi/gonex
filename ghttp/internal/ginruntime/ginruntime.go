// Package ginruntime contains the only ghttp-owned adapter for Gin's
// process-wide configuration. Gin exposes these settings as package globals,
// so callers must serialize changes through RegistrationMu.
package ginruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
)

// RegistrationMu serializes Gin global configuration and route-table
// registration, which both temporarily modify Gin package state.
var RegistrationMu sync.Mutex

// SetMode changes Gin's process-wide mode under the adapter lock.
func SetMode(mode string) {
	RegistrationMu.Lock()
	defer RegistrationMu.Unlock()
	gin.SetMode(mode)
}

// ConfigureLogging installs framework-backed writers and debug callbacks for
// Gin's process-wide logging hooks. The caller must not mutate these hooks
// directly while a registration is in progress.
func ConfigureLogging(logger logging.Logger) {
	RegistrationMu.Lock()
	defer RegistrationMu.Unlock()
	installLogging(logger)
}

// InstallLogging installs Gin's debug callbacks while the caller already owns
// RegistrationMu, which is useful for an atomic route registration.
func InstallLogging(logger logging.Logger) {
	installLogging(logger)
}

// WithQuietLogging temporarily suppresses Gin's debug callbacks. The caller
// must hold RegistrationMu when using this function.
func WithQuietLogging(fn func()) {
	debugRouteFunc := gin.DebugPrintRouteFunc
	debugPrintFunc := gin.DebugPrintFunc
	gin.DebugPrintRouteFunc = func(string, string, string, int) {}
	gin.DebugPrintFunc = func(string, ...any) {}
	defer func() {
		gin.DebugPrintRouteFunc = debugRouteFunc
		gin.DebugPrintFunc = debugPrintFunc
	}()
	fn()
}

func installLogging(logger logging.Logger) {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	ginLogger := logger.Named("gin")
	gin.DefaultWriter = logging.NewWriter(ginLogger, logging.InfoLevel)
	gin.DefaultErrorWriter = logging.NewWriter(ginLogger, logging.ErrorLevel)
	gin.DebugPrintFunc = func(format string, values ...any) {
		message := strings.TrimSpace(fmt.Sprintf(format, values...))
		if message != "" {
			ginLogger.Info(context.Background(), ensureDebugPrefix(message))
		}
	}
	gin.DebugPrintRouteFunc = func(method, path, handler string, numberOfHandlers int) {
		ginLogger.Info(
			context.Background(),
			fmt.Sprintf("[GIN-debug] %-6s %-25s --> %s (%d handlers)", method, path, handler, numberOfHandlers),
			logging.String("path", path),
		)
	}
}

func ensureDebugPrefix(message string) string {
	if strings.HasPrefix(message, "[GIN-debug]") {
		return message
	}
	return "[GIN-debug] " + message
}
