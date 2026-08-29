package router

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Registry stores routes independently of Gin.
type Registry struct {
	mu     sync.RWMutex
	routes []RouteMetadata
	keys   map[string]struct{}
}

// NewRegistry creates an independent route registry.
func NewRegistry() *Registry { return &Registry{keys: make(map[string]struct{})} }

// Validate checks route metadata against the current registry without mutating it.
func (registry *Registry) Validate(routes ...RouteMetadata) error {
	if registry == nil {
		return fmt.Errorf("route registry is nil")
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.validateLocked(routes...)
}

// Register adds route metadata after checking duplicate method/path pairs.
func (registry *Registry) Register(routes ...RouteMetadata) error {
	if registry == nil {
		return fmt.Errorf("route registry is nil")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.keys == nil {
		registry.keys = make(map[string]struct{})
	}
	if err := registry.validateLocked(routes...); err != nil {
		return err
	}
	for _, route := range routes {
		metadata := route
		key := routeKey(metadata.Method, metadata.Path)
		registry.keys[key] = struct{}{}
		registry.routes = append(registry.routes, cloneRouteMetadata(route))
	}
	return nil
}

func (registry *Registry) validateLocked(routes ...RouteMetadata) error {
	pending := make(map[string]struct{}, len(routes))
	for _, metadata := range routes {
		if metadata.Method == "" || metadata.Path == "" {
			return fmt.Errorf("route method and path are required")
		}
		key := routeKey(metadata.Method, metadata.Path)
		if _, exists := registry.keys[key]; exists {
			return fmt.Errorf("route %s %s is already registered", metadata.Method, metadata.Path)
		}
		if _, exists := pending[key]; exists {
			return fmt.Errorf("duplicate route %s %s", metadata.Method, metadata.Path)
		}
		pending[key] = struct{}{}
	}
	return nil
}

// List returns a snapshot of registered routes.
func (registry *Registry) List() []RouteMetadata {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	routes := make([]RouteMetadata, len(registry.routes))
	for index, route := range registry.routes {
		routes[index] = cloneRouteMetadata(route)
	}
	return routes
}

func routeKey(method, path string) string { return strings.ToUpper(method) + " " + path }
func cloneRouteMetadata(route RouteMetadata) RouteMetadata {
	route.Tags = append([]string(nil), route.Tags...)
	route.Security = append([]string(nil), route.Security...)
	route.Produces = append([]string(nil), route.Produces...)
	route.Consumes = append([]string(nil), route.Consumes...)
	route.Bindings = cloneFieldBindings(route.Bindings)
	return route
}

// RouteTable writes a registry table for tools that do not own a Server.
func (registry *Registry) RouteTable(writer io.Writer) error {
	if writer == nil {
		return fmt.Errorf("route table writer is nil")
	}
	if _, err := io.WriteString(writer, fmt.Sprintf("%-8s %-32s %s\n", "METHOD", "ROUTE", "HANDLER")); err != nil {
		return err
	}
	for _, route := range registry.List() {
		metadata := route
		if _, err := io.WriteString(writer, fmt.Sprintf("%-8s %-32s %s.%s\n", metadata.Method, metadata.Path, metadata.ControllerName, metadata.Action)); err != nil {
			return err
		}
	}
	return nil
}
