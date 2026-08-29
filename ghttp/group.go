package ghttp

import (
	"fmt"
	"strings"

	"github.com/lanechi/gonex/router"
)

// RouterGroup scopes route paths and middleware without exposing Gin as the
// route registry's source of truth.
type RouterGroup struct {
	server     *Server
	prefix     string
	middleware []Middleware
	err        error
}

// Middleware adds middleware to this group.
func (group *RouterGroup) Middleware(middleware ...Middleware) *RouterGroup {
	if group == nil {
		return nil
	}
	if err := validateMiddleware(middleware); err != nil {
		if group.err == nil {
			group.err = err
		}
		return group
	}
	group.middleware = append(group.middleware, middleware...)
	return group
}

// Bind scans and registers controllers under the group's path prefix.
func (group *RouterGroup) Bind(controllers ...any) error {
	if group == nil || group.server == nil {
		return fmt.Errorf("route group is nil")
	}
	if group.err != nil {
		return group.err
	}
	if err := group.server.Err(); err != nil {
		return err
	}
	if len(controllers) == 0 {
		return fmt.Errorf("at least one controller is required")
	}
	routes := make([]router.Definition, 0)
	for _, controller := range controllers {
		controllerRoutes, err := scanController(controller)
		if err != nil {
			return err
		}
		for index := range controllerRoutes {
			route := &controllerRoutes[index]
			metadata := route.Metadata
			joinedPath := joinRoutePaths(group.prefix, metadata.Path)
			route.Metadata.Path = joinedPath
		}
		routes = append(routes, controllerRoutes...)
	}
	return group.server.registerRouteDefinitions(routes, group.middleware)
}

// Group creates a path and middleware scope and invokes handler with it.
func (server *Server) Group(prefix string, handler func(*RouterGroup)) {
	group := &RouterGroup{server: server, prefix: prefix}
	if handler != nil {
		handler(group)
	}
}

// Use adds application middleware to all routes registered on the server.
func (server *Server) Use(middleware ...Middleware) error {
	server.registrationMu.Lock()
	defer server.registrationMu.Unlock()
	if server.isRunning() {
		return ErrServerRunning
	}
	if server.routesRegistered {
		return fmt.Errorf("application middleware must be configured before Bind")
	}
	if err := validateMiddleware(middleware); err != nil {
		return fmt.Errorf("application %w", err)
	}
	if len(middleware) == 0 {
		return nil
	}
	server.engine.Use(ginMiddlewareHandlers(middleware)...)
	return nil
}

func joinRoutePaths(prefix, path string) string {
	joined := strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
	if joined == "/" {
		return joined
	}
	if !strings.HasPrefix(joined, "/") {
		return "/" + joined
	}
	return joined
}

func (group *RouterGroup) String() string {
	if group == nil {
		return "<nil>"
	}
	return fmt.Sprintf("RouterGroup(%s)", group.prefix)
}
