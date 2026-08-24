package ghttp

import (
	"fmt"
	"strings"
)

// Route is a pre-registration route scope used to attach middleware to one
// method/path pair without exposing Gin's route registry as the source of
// truth. Configure it before Bind.
type Route struct {
	server *Server
	method string
	path   string
}

// Route returns a route-specific middleware scope. The path is the final
// registered path, including a Group prefix when applicable.
func (server *Server) Route(method, path string) *Route {
	return &Route{server: server, method: strings.ToUpper(strings.TrimSpace(method)), path: strings.TrimSpace(path)}
}

// Use attaches middleware to this route before its Controller is bound.
func (route *Route) Use(middleware ...Middleware) error {
	if route == nil || route.server == nil {
		return fmt.Errorf("route scope is nil")
	}
	if err := validateMiddleware(middleware); err != nil {
		return fmt.Errorf("route %s %s: %w", route.method, route.path, err)
	}
	route.server.registrationMu.Lock()
	defer route.server.registrationMu.Unlock()
	if route.server.isRunning() {
		return ErrServerRunning
	}
	if route.method == "" || route.path == "" || !strings.HasPrefix(route.path, "/") {
		return fmt.Errorf("route method and an absolute path are required")
	}
	for _, registered := range route.server.engine.Routes() {
		if strings.EqualFold(registered.Method, route.method) && routeShape(registered.Path) == routeShape(route.path) {
			return fmt.Errorf("route %s %s is already registered; configure middleware before Bind", route.method, route.path)
		}
	}
	route.server.routeMiddlewareMu.Lock()
	key := routeMiddlewareKey(route.method, route.path)
	route.server.routeMiddleware[key] = append(route.server.routeMiddleware[key], middleware...)
	route.server.routeMiddlewareMu.Unlock()
	return nil
}
