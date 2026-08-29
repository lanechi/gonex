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
	routes []Definition
	keys   map[string]struct{}
}

// NewRegistry creates an independent route registry.
func NewRegistry() *Registry { return &Registry{keys: make(map[string]struct{})} }

// Register adds route definitions after checking duplicate method/path pairs.
func (registry *Registry) Register(routes ...Definition) error {
	if registry == nil {
		return fmt.Errorf("route registry is nil")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.keys == nil {
		registry.keys = make(map[string]struct{})
	}
	pending := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		metadata := route.Metadata
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
	for _, route := range routes {
		metadata := route.Metadata
		key := routeKey(metadata.Method, metadata.Path)
		registry.keys[key] = struct{}{}
		registry.routes = append(registry.routes, cloneDefinition(route))
	}
	return nil
}

// List returns a snapshot of registered routes.
func (registry *Registry) List() []Definition {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	routes := make([]Definition, len(registry.routes))
	for index, route := range registry.routes {
		routes[index] = cloneDefinition(route)
	}
	return routes
}

func routeKey(method, path string) string { return strings.ToUpper(method) + " " + path }
func cloneDefinition(route Definition) Definition {
	route.Metadata.Tags = append([]string(nil), route.Metadata.Tags...)
	route.Metadata.Security = append([]string(nil), route.Metadata.Security...)
	route.Metadata.Produces = append([]string(nil), route.Metadata.Produces...)
	route.Metadata.Consumes = append([]string(nil), route.Metadata.Consumes...)
	route.Metadata.Bindings = cloneFieldBindings(route.Metadata.Bindings)
	if route.Runtime.Binder != nil {
		binder := *route.Runtime.Binder
		binder.Fields = cloneFieldBindings(route.Runtime.Binder.Fields)
		route.Runtime.Binder = &binder
	}
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
		metadata := route.Metadata
		if _, err := io.WriteString(writer, fmt.Sprintf("%-8s %-32s %s.%s\n", metadata.Method, metadata.Path, metadata.ControllerName, metadata.Action)); err != nil {
			return err
		}
	}
	return nil
}
