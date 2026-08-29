package ghttp

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// RouteTable returns a stable, human-readable route table for diagnostics.
func (server *Server) RouteTable() string {
	type routeRow struct {
		method  string
		path    string
		handler string
	}
	frameworkHandlers := make(map[string]string)
	for _, route := range server.Routes() {
		metadata := route.Metadata
		frameworkHandlers[strings.ToUpper(metadata.Method)+" "+metadata.Path] = metadata.ControllerName + "." + metadata.Action
	}
	rows := make([]routeRow, 0, len(server.engine.Routes()))
	for _, route := range server.engine.Routes() {
		handler := frameworkHandlers[strings.ToUpper(route.Method)+" "+route.Path]
		if handler == "" {
			switch {
			case route.Method == "GET" && route.Path == server.openapiPath:
				handler = "OpenAPI"
			case route.Method == "GET" && route.Path == strings.TrimRight(server.swaggerPath, "/")+"/*any":
				handler = "Swagger"
			default:
				handler = route.Handler
			}
		}
		rows = append(rows, routeRow{method: route.Method, path: route.Path, handler: handler})
	}
	if server.staticRootReady {
		rows = append(rows,
			routeRow{method: "GET", path: "/*filepath", handler: "Static fallback"},
			routeRow{method: "HEAD", path: "/*filepath", handler: "Static fallback"},
		)
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].path == rows[right].path {
			return rows[left].method < rows[right].method
		}
		return rows[left].path < rows[right].path
	})
	var builder strings.Builder
	fmt.Fprintf(&builder, "%-8s %-32s %s\n", "METHOD", "ROUTE", "HANDLER")
	for _, route := range rows {
		fmt.Fprintf(&builder, "%-8s %-32s %s\n", route.method, route.path, route.handler)
	}
	return builder.String()
}

// PrintRoutes writes the framework route table to writer.
func (server *Server) PrintRoutes(writer io.Writer) error {
	if writer == nil {
		return fmt.Errorf("route table writer is nil")
	}
	_, err := io.WriteString(writer, server.RouteTable())
	return err
}
