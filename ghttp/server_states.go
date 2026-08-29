package ghttp

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/lifecycle"
	"github.com/lanechi/gonex/openapi"
	"github.com/lanechi/gonex/router"
	"github.com/lanechi/gonex/scheduler"
)

// routingState owns the mutable route execution and registration state.
type routingState struct {
	engine            *gin.Engine
	httpServer        *http.Server
	registry          *router.Registry
	registrationMu    sync.Mutex
	routesRegistered  bool
	routeMiddlewareMu sync.RWMutex
	routeMiddleware   map[string][]Middleware
}

// docsState owns generated documentation settings and its cache.
type docsState struct {
	openapiEnabled    bool
	openapiPath       string
	swaggerPath       string
	openapiRouteReady bool
	swaggerRouteReady bool
	openapiMu         sync.RWMutex
	openapiCache      *openapi.Document
}

// securityState owns request security and session configuration.
type securityState struct {
	settingsMu           sync.RWMutex
	sessionManager       *SessionManager
	sessionCookieOptions *CookieOptions
	allowedHosts         []string
	csrfOptions          *CSRFOptions
	corsOptions          *CORSOptions
	corsHandler          gin.HandlerFunc
	csrfHandler          gin.HandlerFunc
}

type serverShutdownAttempt struct {
	done chan struct{}
	err  error
}

// runtimeState owns lifecycle, listener, and construction status.
type runtimeState struct {
	restartManager    RestartManager
	lifecycle         *lifecycle.Lifecycle
	scheduler         scheduler.Scheduler
	schedulerEnabled  bool
	schedulerLocation *time.Location
	listenerMu        sync.RWMutex
	listener          net.Listener
	initializationErr error
	stateMu           sync.RWMutex
	running           bool
	staticRootReady   bool
	shutdownMu        sync.Mutex
	shutdownAttempt   *serverShutdownAttempt
}
