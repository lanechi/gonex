package ghttp

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
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
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register Gin route: %v", recovered)
		}
	}()
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	installGinLogging(server.logger)
	for _, route := range routes {
		route.Binder.MaxMultipartMemory = server.maxMultipartMemory
		routeMiddleware := server.middlewareForRoute(route.Method, route.Path)
		handlers := make([]gin.HandlerFunc, 0, len(routeMiddleware)+len(middleware)+1)
		handlers = append(handlers, ginMiddlewareHandlers(middleware)...)
		handlers = append(handlers, ginMiddlewareHandlers(routeMiddleware)...)
		handlers = append(handlers, server.handlerFor(route))
		server.engine.Handle(route.Method, route.Path, handlers...)
	}
	if err := server.registry.Register(routes...); err != nil {
		return err
	}
	server.routesRegistered = true
	server.invalidateOpenAPI()
	return nil
}

func (server *Server) validateRouteHandlerCounts(routes []router.Definition, middleware []Middleware) error {
	globalHandlers := len(server.engine.Handlers)
	for _, route := range routes {
		routeHandlers := len(server.middlewareForRoute(route.Method, route.Path))
		total := globalHandlers + len(middleware) + routeHandlers + 1
		if total >= ginAbortIndex {
			return fmt.Errorf("route %s %s has %d handlers; Gin allows at most %d", route.Method, route.Path, total, ginAbortIndex-1)
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
		if route.Binder == nil {
			return fmt.Errorf("route %s %s has no request binder", route.Method, route.Path)
		}
		if err := router.ValidatePathBindings(route.Path, route.Binder.Fields); err != nil {
			return fmt.Errorf("route %s %s: %w", route.Method, route.Path, err)
		}
		key := strings.ToUpper(route.Method) + " " + routeShape(route.Path)
		if path, exists := existing[key]; exists {
			return fmt.Errorf("route %s %s conflicts with registered route %s", route.Method, route.Path, path)
		}
		if path, exists := pending[key]; exists {
			return fmt.Errorf("route %s %s conflicts with controller route %s", route.Method, route.Path, path)
		}
		pending[key] = route.Path
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
	debugRouteFunc := gin.DebugPrintRouteFunc
	debugPrintFunc := gin.DebugPrintFunc
	gin.DebugPrintRouteFunc = func(string, string, string, int) {}
	gin.DebugPrintFunc = func(string, ...any) {}
	defer func() {
		gin.DebugPrintRouteFunc = debugRouteFunc
		gin.DebugPrintFunc = debugPrintFunc
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("route table conflict: %v", recovered)
		}
	}()
	engine := gin.New()
	placeholder := func(*gin.Context) {}
	for _, route := range existing {
		engine.Handle(route.Method, route.Path, placeholder)
	}
	for _, route := range pending {
		engine.Handle(route.Method, route.Path, placeholder)
	}
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
