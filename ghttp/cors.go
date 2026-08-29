package ghttp

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSOptions configures cross-origin requests.
type CORSOptions struct {
	Enabled          bool
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

func (server *Server) EnableCORS(options CORSOptions) error {
	options = cloneCORSOptions(options)
	var handler gin.HandlerFunc
	var err error
	if options.Enabled {
		handler, err = newCORSHandler(options)
		if err != nil {
			return err
		}
	}

	server.settingsMu.Lock()
	defer server.settingsMu.Unlock()
	server.options.CORS = optional[CORSOptions]{Value: cloneCORSOptions(options), Set: true}
	if !options.Enabled {
		server.corsOptions = nil
		server.corsHandler = nil
		return nil
	}
	copy := cloneCORSOptions(options)
	server.corsOptions = &copy
	server.corsHandler = handler
	return nil
}

func (server *Server) configureCORSHandler() error {
	server.settingsMu.Lock()
	defer server.settingsMu.Unlock()
	if server.corsOptions == nil || !server.corsOptions.Enabled {
		server.corsHandler = nil
		return nil
	}
	options := cloneCORSOptions(*server.corsOptions)
	handler, err := newCORSHandler(options)
	if err != nil {
		return err
	}
	server.corsOptions = &options
	server.options.CORS = optional[CORSOptions]{Value: cloneCORSOptions(options), Set: true}
	server.corsHandler = handler
	return nil
}

func corsMiddleware(server *Server) gin.HandlerFunc {
	return func(context *gin.Context) {
		server.settingsMu.RLock()
		handler := server.corsHandler
		server.settingsMu.RUnlock()
		if handler == nil {
			context.Next()
			return
		}
		handler(context)
	}
}

func newCORSHandler(options CORSOptions) (gin.HandlerFunc, error) {
	options = cloneCORSOptions(options)
	configuration := cors.Config{
		AllowOrigins:     options.AllowOrigins,
		AllowMethods:     options.AllowMethods,
		AllowHeaders:     options.AllowHeaders,
		ExposeHeaders:    options.ExposeHeaders,
		AllowCredentials: options.AllowCredentials,
	}
	if options.MaxAge > 0 {
		configuration.MaxAge = time.Duration(options.MaxAge) * time.Second
	}
	if len(configuration.AllowOrigins) == 0 {
		return nil, fmt.Errorf("CORS requires at least one allowed origin when enabled")
	}
	if len(configuration.AllowMethods) == 0 {
		configuration.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(configuration.AllowHeaders) == 0 {
		configuration.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", requestIDHeader}
	}
	if configuration.AllowCredentials {
		for _, origin := range configuration.AllowOrigins {
			if strings.TrimSpace(origin) == "*" {
				return nil, fmt.Errorf("CORS credentials cannot be combined with wildcard origins")
			}
		}
	}
	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid CORS configuration: %w", err)
	}
	return cors.New(configuration), nil
}

func cloneCORSOptions(options CORSOptions) CORSOptions {
	options.AllowOrigins = append([]string(nil), options.AllowOrigins...)
	options.AllowMethods = append([]string(nil), options.AllowMethods...)
	options.AllowHeaders = append([]string(nil), options.AllowHeaders...)
	options.ExposeHeaders = append([]string(nil), options.ExposeHeaders...)
	return options
}
