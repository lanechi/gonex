package ghttp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/openapi"
)

func (server *Server) swaggerHandler(context *gin.Context) {
	server.settingsMu.RLock()
	enabled := server.openapiEnabled
	server.settingsMu.RUnlock()
	if !enabled {
		context.Status(http.StatusNotFound)
		return
	}
	context.Header("X-Content-Type-Options", "nosniff")
	context.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'unsafe-inline'; connect-src 'self'")
	context.Data(http.StatusOK, "text/html; charset=utf-8", openapi.RenderSwaggerHTML(server.openapiPath))
}
