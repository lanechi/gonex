package ghttp

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/ghttp/internal/ginruntime"
	"github.com/lanechi/gonex/router"
)

// Gin reserves index 63 as the abort sentinel, so a route may contain at most
// 62 global and route-local handlers in total.
const ginAbortIndex = 63

func (server *Server) registerRouteDefinitions(routes []router.Definition, middleware []Middleware) (err error) {
	if err := validateMiddleware(middleware); err != nil {
		return fmt.Errorf("route %w", err)
	}
	server.registrationMu.Lock()
	defer server.registrationMu.Unlock()
	if server.isRunning() {
		return ErrServerRunning
	}
	if err := server.validateRouteHandlerCounts(routes, middleware); err != nil {
		return err
	}
	if err := server.validateRouteRegistration(routes); err != nil {
		return err
	}
	metadata := make([]router.RouteMetadata, len(routes))
	for index, route := range routes {
		metadata[index] = route.Metadata
	}
	if err := server.registry.Validate(metadata...); err != nil {
		return fmt.Errorf("route registry: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register Gin route: %v", recovered)
		}
	}()
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	installGinLogging(server.logger)
	for _, route := range routes {
		metadata := route.Metadata
		runtime := route.Runtime
		runtime.Binder.MaxMultipartMemory = server.maxMultipartMemory
		routeMiddleware := server.middlewareForRoute(metadata.Method, metadata.Path)
		handlers := make([]gin.HandlerFunc, 0, len(routeMiddleware)+len(middleware)+1)
		handlers = append(handlers, ginMiddlewareHandlers(middleware)...)
		handlers = append(handlers, ginMiddlewareHandlers(routeMiddleware)...)
		handlers = append(handlers, server.handlerFor(route))
		server.engine.Handle(metadata.Method, metadata.Path, handlers...)
	}
	if err := server.registry.Register(metadata...); err != nil {
		return err
	}
	server.routesRegistered = true
	server.invalidateOpenAPI()
	return nil
}

func (server *Server) validateRouteHandlerCounts(routes []router.Definition, middleware []Middleware) error {
	globalHandlers := len(server.engine.Handlers)
	for _, route := range routes {
		metadata := route.Metadata
		routeHandlers := len(server.middlewareForRoute(metadata.Method, metadata.Path))
		total := globalHandlers + len(middleware) + routeHandlers + 1
		if total >= ginAbortIndex {
			return fmt.Errorf("route %s %s has %d handlers; Gin allows at most %d", metadata.Method, metadata.Path, total, ginAbortIndex-1)
		}
	}
	return nil
}

func (server *Server) middlewareForRoute(method, path string) []Middleware {
	server.routeMiddlewareMu.RLock()
	defer server.routeMiddlewareMu.RUnlock()
	return append([]Middleware(nil), server.routeMiddleware[routeMiddlewareKey(method, path)]...)
}

func routeMiddlewareKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + routeShape(path)
}

func (server *Server) validateRouteRegistration(routes []router.Definition) error {
	existing := make(map[string]string)
	for _, route := range server.engine.Routes() {
		key := strings.ToUpper(route.Method) + " " + routeShape(route.Path)
		existing[key] = route.Path
	}
	pending := make(map[string]string, len(routes))
	for _, route := range routes {
		metadata := route.Metadata
		runtime := route.Runtime
		if runtime.Binder == nil {
			return fmt.Errorf("route %s %s has no request binder", metadata.Method, metadata.Path)
		}
		if err := router.ValidatePathBindings(metadata.Path, metadata.Bindings); err != nil {
			return fmt.Errorf("route %s %s: %w", metadata.Method, metadata.Path, err)
		}
		key := strings.ToUpper(metadata.Method) + " " + routeShape(metadata.Path)
		if path, exists := existing[key]; exists {
			return fmt.Errorf("route %s %s conflicts with registered route %s", metadata.Method, metadata.Path, path)
		}
		if path, exists := pending[key]; exists {
			return fmt.Errorf("route %s %s conflicts with controller route %s", metadata.Method, metadata.Path, path)
		}
		pending[key] = metadata.Path
	}
	if err := validateGinRouteTable(server.engine.Routes(), routes); err != nil {
		return err
	}
	return nil
}

// validateGinRouteTable mirrors the registration order in a temporary Gin
// engine. Gin's radix tree rejects conflicts that are more subtle than a
// simple method/path string comparison, such as a parameter route competing
// with a catch-all route. Checking before mutating the real engine keeps a
// failed batch registration atomic from the framework's point of view.
func validateGinRouteTable(existing []gin.RouteInfo, pending []router.Definition) (err error) {
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("route table conflict: %v", recovered)
		}
	}()
	var engine *gin.Engine
	ginruntime.WithQuietLogging(func() {
		engine = gin.New()
		placeholder := func(*gin.Context) {}
		for _, route := range existing {
			engine.Handle(route.Method, route.Path, placeholder)
		}
		for _, route := range pending {
			metadata := route.Metadata
			engine.Handle(metadata.Method, metadata.Path, placeholder)
		}
	})
	return nil
}

func routeShape(routePath string) string {
	segments := strings.Split(routePath, "/")
	for index, segment := range segments {
		if len(segment) > 1 && segment[0] == ':' {
			segments[index] = ":"
		} else if len(segment) > 1 && segment[0] == '*' {
			segments[index] = "*"
		}
	}
	return strings.Join(segments, "/")
}

func (server *Server) registerGinGET(path string, handler gin.HandlerFunc) (err error) {
	for _, route := range server.engine.Routes() {
		if route.Method == "GET" && routeShape(route.Path) == routeShape(path) {
			return fmt.Errorf("GET %s conflicts with registered route %s", path, route.Path)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("GET %s: %v", path, recovered)
		}
	}()
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	installGinLogging(server.logger)
	server.engine.GET(path, handler)
	return nil
}
