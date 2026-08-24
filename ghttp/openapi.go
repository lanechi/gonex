package ghttp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/openapi"
)

// OpenAPI builds a cached document from the framework route registry.
func (server *Server) OpenAPI() openapi.Document {
	server.openapiMu.RLock()
	if server.openapiCache != nil {
		document := openapi.Clone(*server.openapiCache)
		server.openapiMu.RUnlock()
		return document
	}
	server.openapiMu.RUnlock()

	server.openapiMu.Lock()
	defer server.openapiMu.Unlock()
	if server.openapiCache == nil {
		document := openapi.Generate(server.name, server.Routes())
		server.openapiCache = &document
	}
	return openapi.Clone(*server.openapiCache)
}

// OpenAPIJSON returns the generated document as indented JSON.
func (server *Server) OpenAPIJSON() ([]byte, error) {
	return json.MarshalIndent(server.OpenAPI(), "", "  ")
}

func (server *Server) openAPIHandler(context *gin.Context) {
	server.settingsMu.RLock()
	enabled := server.openapiEnabled
	server.settingsMu.RUnlock()
	if !enabled {
		context.Status(http.StatusNotFound)
		return
	}
	context.JSON(http.StatusOK, server.OpenAPI())
}
