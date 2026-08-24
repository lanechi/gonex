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
		if route.Method == "" || route.Path == "" {
			return fmt.Errorf("route method and path are required")
		}
		key := routeKey(route.Method, route.Path)
		if _, exists := registry.keys[key]; exists {
			return fmt.Errorf("route %s %s is already registered", route.Method, route.Path)
		}
		if _, exists := pending[key]; exists {
			return fmt.Errorf("duplicate route %s %s", route.Method, route.Path)
		}
		pending[key] = struct{}{}
	}
	for _, route := range routes {
		key := routeKey(route.Method, route.Path)
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
	route.Tags = append([]string(nil), route.Tags...)
	route.Security = append([]string(nil), route.Security...)
	route.Produces = append([]string(nil), route.Produces...)
	route.Consumes = append([]string(nil), route.Consumes...)
	if route.Binder != nil {
		binder := *route.Binder
		binder.Fields = append([]FieldBinding(nil), route.Binder.Fields...)
		for index := range binder.Fields {
			binder.Fields[index].Index = append([]int(nil), binder.Fields[index].Index...)
		}
		route.Binder = &binder
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
		if _, err := io.WriteString(writer, fmt.Sprintf("%-8s %-32s %s.%s\n", route.Method, route.Path, route.ControllerName, route.Action)); err != nil {
			return err
		}
	}
	return nil
}
