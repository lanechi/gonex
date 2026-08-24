package g

import (
	"errors"
	"strings"
	"sync"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

// Meta marks a request structure as a declarative route definition.
type Meta struct{}

var (
	serverRegistryMu sync.Mutex
	serverRegistry   = make(map[string]*ghttp.Server)
)

// SetLogger registers a custom Logger before the framework creates its first
// Server. The logger must implement logging.Logger; this makes the framework
// independent of Zap, Logrus, Zerolog, or an application-defined backend.
//
// Existing Servers are not changed. Passing nil clears the pre-initialization
// logger registration.
func SetLogger(logger logging.Logger) error {
	serverRegistryMu.Lock()
	defer serverRegistryMu.Unlock()
	if len(serverRegistry) > 0 {
		return errors.New("gonex logger must be configured before the first Server is initialized")
	}
	logging.SetLogger(logger)
	return nil
}

// Server returns a process-wide named server.
//
// Calling Server() returns the default server. Named servers are independent
// instances and are cached by name, so Server("api") always returns the same
// server while Server("api") and Server("admin") return different servers.
// Use ghttp.NewServer directly when an uncached server instance is required.
func Server(names ...string) *ghttp.Server {
	name := "default"
	if len(names) > 0 && strings.TrimSpace(names[0]) != "" {
		name = strings.TrimSpace(names[0])
	}

	serverRegistryMu.Lock()
	defer serverRegistryMu.Unlock()
	if server, ok := serverRegistry[name]; ok {
		return server
	}

	options := make([]ghttp.Option, 0, 1)
	if name != "default" {
		options = append(options, ghttp.WithName(name))
	}
	server := ghttp.NewServer(options...)
	serverRegistry[name] = server
	return server
}

// Cfg returns the process-wide project configuration. It loads the root
// .env and the conventional config file lazily on first use.
func Cfg() *config.ViperConfig {
	return config.Default()
}
