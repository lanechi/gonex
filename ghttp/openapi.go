package ghttp

import (
	"encoding/json"
	"net/http"
	"strings"

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
		document := openapi.Generate(server.openAPIInfo(), server.Routes())
		server.openapiCache = &document
	}
	return openapi.Clone(*server.openapiCache)
}

func (server *Server) openAPIInfo() openapi.Info {
	info := openapi.Info{Title: server.name, Version: "unversioned"}
	if server.config == nil {
		return info
	}
	if title := strings.TrimSpace(server.config.GetString("server.openapi.title")); title != "" {
		info.Title = title
	}
	if version := strings.TrimSpace(server.config.GetString("server.openapi.version")); version != "" {
		info.Version = version
	}
	return info
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
