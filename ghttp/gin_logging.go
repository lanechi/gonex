package ghttp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
)

var ginRouteRegistrationMu sync.Mutex

const Named = "[GIN-debug]"

// configureGinLogging routes Gin's two package-level writers through the
// framework logger. Gin is an internal implementation detail of ghttp, so no
// Gin adapter is exposed from logging.
func configureGinLogging(logger logging.Logger) {
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	ginLogger := logger.Named("gin")
	gin.DefaultWriter = logging.NewWriter(ginLogger, logging.InfoLevel)
	gin.DefaultErrorWriter = logging.NewWriter(ginLogger, logging.ErrorLevel)
	installGinLogging(logger)
}

func installGinLogging(logger logging.Logger) {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	ginLogger := logger.Named("gin")
	gin.DebugPrintFunc = func(format string, values ...any) {
		message := strings.TrimSpace(fmt.Sprintf(format, values...))
		if message != "" {
			ginLogger.Info(context.Background(), ensureGinDebugPrefix(message))
		}
	}
	gin.DebugPrintRouteFunc = func(method, path, handler string, numberOfHandlers int) {
		ginLogger.Info(
			context.Background(),
			fmt.Sprintf("%s %-6s %-25s --> %s (%d handlers)", Named, method, path, handler, numberOfHandlers),
			logging.String("path", path),
		)
	}
}

func ensureGinDebugPrefix(message string) string {
	if strings.HasPrefix(message, Named) {
		return message
	}
	return Named + " " + message
}
