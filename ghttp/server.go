package ghttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	gonexconfig "github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/lifecycle"
	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/middleware"
	"github.com/lanechi/gonex/router"
	"github.com/lanechi/gonex/scheduler"
	"github.com/lanechi/gonex/template"
)

const defaultAddress = ":8000"

// Server is the framework's HTTP server and route owner.
type Server struct {
	name    string
	address string
	options serverOptions

	routingState
	docsState
	securityState
	runtimeState

	responseEncoder         ResponseEncoder
	errorHandler            ErrorHandler
	bindingValidator        *validator.Validate
	validateValidator       *validator.Validate
	customBindingValidator  bool
	customValidateValidator bool
	logger                  logging.Logger
	config                  Config
	mode                    string
	debug                   bool
	templates               *template.Manager
	readTimeout             time.Duration
	writeTimeout            time.Duration
	idleTimeout             time.Duration
	maxBodyBytes            int64
	maxMultipartMemory      int64
	maxHeaderBytes          int
	shutdownTimeout         time.Duration
	tlsEnabled              bool
	tlsCertFile             string
	tlsKeyFile              string
	requestIDEnabled        bool
}

// NewServer creates an independent Server instance.
func NewServer(options ...Option) *Server {
	defaultConfiguration := gonexconfig.Default()
	defaultConfigurationErr := gonexconfig.Init()
	server := newServerDefaults(defaultConfiguration)
	for _, option := range options {
		if option != nil {
			option(server)
		}
	}
	return server.initialize(defaultConfigurationErr)
}

func newServerDefaults(configuration Config) *Server {
	server := &Server{
		name:               "default",
		address:            defaultAddress,
		readTimeout:        15 * time.Second,
		writeTimeout:       15 * time.Second,
		idleTimeout:        60 * time.Second,
		maxBodyBytes:       10 << 20,
		maxMultipartMemory: 32 << 20,
		maxHeaderBytes:     1 << 20,
		shutdownTimeout:    10 * time.Second,
		requestIDEnabled:   true,
		config:             configuration,
		routingState:       routingState{registry: router.NewRegistry(), routeMiddleware: make(map[string][]Middleware)},
		docsState:          docsState{openapiEnabled: true, openapiPath: "/openapi.json", swaggerPath: "/docs"},
	}
	if logger := logging.InitialLogger(); logger != nil {
		server.logger = logger
		server.options.Logger = optional[logging.Logger]{Value: server.logger, Set: true}
	} else {
		server.logger = logging.NewDefaultLogger()
	}
	server.sessionManager = NewSessionManager(nil, "session_id", 24*time.Hour)
	server.templates = template.New()
	server.restartManager = &serverRestartManager{server: server}
	server.lifecycle = lifecycle.New()
	server.schedulerEnabled = true
	server.responseEncoder = DefaultResponseEncoder{}
	server.errorHandler = defaultErrorHandler
	server.bindingValidator = newValidator("binding")
	server.validateValidator = newValidator("validate")
	return server
}

func (server *Server) initialize(defaultConfigurationErr error) *Server {
	if server.customBindingValidator && server.customValidateValidator && server.bindingValidator == server.validateValidator {
		server.addInitializationError(fmt.Errorf("binding and validate validators must be independent instances"))
	}
	if !server.options.Config.Set && defaultConfigurationErr != nil {
		server.addInitializationError(defaultConfigurationErr)
	}
	server.applyLoggerConfig()
	server.applyModeConfig()
	setGinMode(server.mode)
	if server.engine == nil {
		server.engine = gin.New()
		configureGinLogging(server.logger)
	} else {
		configureGinLogging(server.logger)
	}
	// Gin otherwise trusts all proxies. The framework default is safer: only
	// explicitly configured proxies are trusted.
	_ = server.engine.SetTrustedProxies(nil)
	server.applyConfig()
	server.configureScheduler()
	if server.sessionCookieOptions != nil && server.sessionManager != nil {
		server.sessionManager.SetCookieOptions(*server.sessionCookieOptions)
	}
	if server.sessionManager != nil {
		cookieOptions := server.sessionManager.CookieOptions()
		if cookieOptions.SameSite == http.SameSiteNoneMode && !cookieOptions.Secure {
			server.addInitializationError(fmt.Errorf("session SameSite=None requires a Secure cookie"))
		}
	}
	server.httpServer = &http.Server{
		Addr:           server.address,
		Handler:        server,
		ErrorLog:       logging.NewStdLogger(server.logger.Named("net/http"), logging.ErrorLevel),
		ReadTimeout:    server.readTimeout,
		WriteTimeout:   server.writeTimeout,
		IdleTimeout:    server.idleTimeout,
		MaxHeaderBytes: server.maxHeaderBytes,
	}
	server.registerValidatorNames()
	server.engine.Use(
		requestIDMiddleware(server),
		requestLoggerMiddleware(server),
		frameworkContextMiddleware(server),
		accessLogMiddleware(server),
		recoveryMiddleware(server),
		hostValidationMiddleware(server),
		requestBodyLimitMiddleware(server),
	)
	if err := server.configureCORSHandler(); err != nil {
		server.addInitializationError(err)
	}
	if err := server.configureCSRFHandler(); err != nil {
		server.addInitializationError(err)
	}
	server.engine.Use(corsMiddleware(server), csrfMiddleware(server))
	server.configureStaticFromConfig()
	server.registerDocumentationRoutes()
	return server
}

// Bind scans a controller, creates route definitions, and registers those
// definitions with both the framework registry and Gin.
func (server *Server) Bind(controller any, middleware ...Middleware) error {
	if err := server.Err(); err != nil {
		return err
	}
	routes, err := scanController(controller)
	if err != nil {
		return err
	}
	return server.registerRouteDefinitions(routes, middleware)
}

func (server *Server) invalidateOpenAPI() {
	server.openapiMu.Lock()
	server.openapiCache = nil
	server.openapiMu.Unlock()
}

// MustBind is the startup-oriented variant of Bind. It panics when a
// controller does not satisfy the declared route contract.
func (server *Server) MustBind(controller any) {
	if err := server.Bind(controller); err != nil {
		panic(err)
	}
}

// Engine exposes the underlying Gin engine for controlled integrations.
func (server *Server) Engine() *gin.Engine {
	return server.engine
}

// Routes returns a snapshot of the framework-owned route table.
func (server *Server) Routes() []router.RouteMetadata {
	return server.registry.List()
}

// Name returns the server's logical name.
func (server *Server) Name() string {
	return server.name
}

// Address returns the configured listening address.
func (server *Server) Address() string {
	return server.address
}

// HTTPServer returns the underlying net/http server for advanced integrations.
func (server *Server) HTTPServer() *http.Server {
	return server.httpServer
}

// Config returns the attached application configuration.
func (server *Server) Config() Config {
	return server.config
}

// Logger returns the server's framework logger.
func (server *Server) Logger() logging.Logger {
	if server == nil {
		return nil
	}
	return server.logger
}

// Scheduler returns the Server-owned scheduler. It starts before the HTTP
// listener and stops during graceful shutdown; each Server has an independent
// scheduler instance unless an application intentionally supplies one.
func (server *Server) Scheduler() scheduler.Scheduler {
	if server == nil {
		return nil
	}
	return server.scheduler
}

// SessionManager returns the configured session manager.
func (server *Server) SessionManager() *SessionManager {
	return server.sessionManager
}

// RestartManager returns the configured restart implementation.
func (server *Server) RestartManager() RestartManager {
	return server.restartManager
}

// SetTemplateRoot configures and loads the server template directory.
func (server *Server) SetTemplateRoot(root string) error {
	server.options.TemplateRoot = optional[string]{Value: root, Set: true}
	return server.templates.SetRoot(root)
}

// SetTrustedProxies configures which proxies Gin may trust for client IP
// resolution. Passing nil disables proxy trust.
func (server *Server) SetTrustedProxies(proxies []string) error {
	if server.isRunning() {
		return ErrServerRunning
	}
	return server.engine.SetTrustedProxies(proxies)
}

// SetAllowedHosts limits Host headers accepted by the HTTP server. An empty
// list disables host validation. Entries may be exact hosts or *.example.com.
func (server *Server) SetAllowedHosts(hosts ...string) {
	server.settingsMu.Lock()
	defer server.settingsMu.Unlock()
	server.options.AllowedHosts = optional[[]string]{Value: append([]string(nil), hosts...), Set: true}
	server.allowedHosts = append([]string(nil), hosts...)
}

// EnableCSRF enables double-submit-cookie CSRF protection for unsafe methods.
func (server *Server) EnableCSRF(options CSRFOptions) error {
	if err := validateCSRFOptions(options); err != nil {
		return err
	}
	server.settingsMu.Lock()
	defer server.settingsMu.Unlock()
	server.options.CSRF = optional[CSRFOptions]{Value: options, Set: true}
	if options.Enabled {
		copy := options
		server.csrfOptions = &copy
		server.csrfHandler = middleware.CSRF(toMiddlewareCSRFOptions(copy))
	} else {
		server.csrfOptions = nil
		server.csrfHandler = nil
	}
	return nil
}

func (server *Server) configureCSRFHandler() error {
	server.settingsMu.Lock()
	defer server.settingsMu.Unlock()
	if server.csrfOptions == nil || !server.csrfOptions.Enabled {
		server.csrfHandler = nil
		return nil
	}
	if err := validateCSRFOptions(*server.csrfOptions); err != nil {
		return err
	}
	server.csrfHandler = middleware.CSRF(toMiddlewareCSRFOptions(*server.csrfOptions))
	return nil
}

func validateCSRFOptions(options CSRFOptions) error {
	if !options.Enabled {
		return nil
	}
	if options.SameSite == http.SameSiteNoneMode && !options.Secure {
		return fmt.Errorf("CSRF SameSite=None requires csrf.secure=true")
	}
	cookieName := options.CookieName
	if cookieName == "" {
		cookieName = "csrf_token"
	}
	if err := (&http.Cookie{Name: cookieName, Value: "token"}).Valid(); err != nil {
		return fmt.Errorf("invalid CSRF cookie name: %w", err)
	}
	headerName := options.HeaderName
	if headerName == "" {
		headerName = "X-CSRF-Token"
	}
	for _, character := range headerName {
		if character <= 32 || character >= 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={}\t", character) {
			return fmt.Errorf("invalid CSRF header name %q", headerName)
		}
	}
	return nil
}

// AddTemplateFunc registers a function available to templates.
func (server *Server) AddTemplateFunc(name string, function any) error {
	return server.templates.AddFunc(name, function)
}

// Templates returns the server template manager.
func (server *Server) Templates() *template.Manager {
	return server.templates
}

func (server *Server) beginRun() error {
	server.registrationMu.Lock()
	defer server.registrationMu.Unlock()
	server.stateMu.Lock()
	defer server.stateMu.Unlock()
	if server.running {
		return ErrServerRunning
	}
	server.running = true
	return nil
}

func (server *Server) endRun() {
	server.stateMu.Lock()
	server.running = false
	server.stateMu.Unlock()
}

func (server *Server) isRunning() bool {
	server.stateMu.RLock()
	defer server.stateMu.RUnlock()
	return server.running
}

// ServeHTTP makes Server usable with httptest and net/http adapters.
func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if server.Err() != nil {
		http.Error(writer, "internal server error", http.StatusInternalServerError)
		return
	}
	server.engine.ServeHTTP(writer, request)
}

// Err returns a construction or configuration error that prevents the server
// from starting safely.
func (server *Server) Err() error {
	if server == nil {
		return errors.New("server is nil")
	}
	return server.initializationErr
}

func (server *Server) addInitializationError(err error) {
	if err != nil {
		server.initializationErr = errors.Join(server.initializationErr, err)
	}
}

// Run starts the HTTP server and blocks until it stops. An optional address
// overrides the configured listening address, so both Run() and Run(":8001")
// are supported.
func (server *Server) Run(address ...string) error {
	if len(address) > 1 {
		return fmt.Errorf("Run accepts at most one address")
	}
	if len(address) == 1 {
		if err := server.setAddress(address[0]); err != nil {
			return err
		}
	}
	return server.runWithSignals(server.tlsEnabled, server.tlsCertFile, server.tlsKeyFile)
}

func (server *Server) setAddress(address string) error {
	if server.isRunning() {
		return ErrServerRunning
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil
	}
	server.address = address
	server.httpServer.Addr = address
	return nil
}

// logListening emits the same useful startup line users get from Gin's
// Engine.Run. The logger controls whether the line is visible, so custom
// logger implementations and logger.enabled=false behave consistently.
func (server *Server) logListening(tlsEnabled bool) {
	if !server.IsDebug() || !server.logger.Enabled(logging.InfoLevel) {
		return
	}
	protocol := "HTTP"
	if tlsEnabled {
		protocol = "HTTPS"
	}
	address := server.httpServer.Addr
	if strings.TrimSpace(address) == "" {
		address = "<listener>"
	}
	server.logger.Named("server").Info(
		context.Background(),
		"server listening",
		logging.String("protocol", protocol),
		logging.String("addr", address),
	)
}

// Shutdown gracefully stops the HTTP server.
func (server *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycleErr := server.lifecycle.BeginShutdown(ctx)
	shutdownErr := server.httpServer.Shutdown(ctx)
	templateErr := server.templates.Close()
	if errors.Is(shutdownErr, http.ErrServerClosed) {
		shutdownErr = nil
	}
	if shutdownErr == nil {
		shutdownErr = templateErr
	}
	if server.scheduler != nil {
		if schedulerErr := server.scheduler.Wait(ctx); shutdownErr == nil {
			shutdownErr = schedulerErr
		}
	}
	if taskErr := server.lifecycle.Wait(ctx); shutdownErr == nil {
		shutdownErr = taskErr
	}
	_ = server.logger.Sync()
	if stopErr := server.lifecycle.Stop(ctx); shutdownErr == nil {
		shutdownErr = stopErr
	}
	if shutdownErr == nil {
		shutdownErr = lifecycleErr
	}
	return shutdownErr
}

// EnableOpenAPI enables or disables the interface documentation feature,
// including both the generated OpenAPI JSON and Swagger UI endpoints.
func (server *Server) EnableOpenAPI(enabled bool) {
	server.settingsMu.Lock()
	server.openapiEnabled = enabled
	server.options.OpenAPI = optional[OpenAPIOptions]{Value: OpenAPIOptions{Enabled: enabled}, Set: true}
	server.settingsMu.Unlock()
}

func (server *Server) registerDocumentationRoutes() {
	if !server.openapiRouteReady {
		if err := server.registerGinGET(server.openapiPath, server.openAPIHandler); err != nil {
			server.addInitializationError(fmt.Errorf("register OpenAPI route: %w", err))
		} else {
			server.openapiRouteReady = true
		}
	}
	if !server.swaggerRouteReady {
		path := strings.TrimRight(server.swaggerPath, "/")
		if path == "" {
			path = "/docs"
		}
		if err := server.registerGinGET(path+"/*any", server.swaggerHandler); err != nil {
			server.addInitializationError(fmt.Errorf("register Swagger route: %w", err))
		} else {
			server.swaggerRouteReady = true
		}
	}
}

// Listen starts the configured HTTP server on address.
func (server *Server) Listen(address string) error {
	if err := server.setAddress(address); err != nil {
		return err
	}
	return server.Run()
}

// ListenTLS starts the configured HTTPS server on address.
func (server *Server) ListenTLS(address, certFile, keyFile string) error {
	if err := server.setAddress(address); err != nil {
		return err
	}
	return server.RunTLS(certFile, keyFile)
}

func (server *Server) configureStaticFromConfig() {
	if server.config == nil {
		return
	}
	if enabled, ok := configBool(server.config.Get("server.static.enabled")); !ok || !enabled {
		return
	}
	if root := configString(server.config.Get("server.static.root")); root != "" {
		if err := server.StaticWithOptions("/static", root, StaticOptions{
			CacheControl: configString(server.config.Get("server.static.cacheControl")),
			Index:        configString(server.config.Get("server.static.index")),
			SPAFallback:  configBoolValue(server.config.Get("server.static.spaFallback")),
			Extensions:   configStrings(server.config.Get("server.static.extensions")),
		}); err != nil {
			server.addInitializationError(fmt.Errorf("configure static root: %w", err))
		}
	}
	var values struct {
		Server struct {
			Static struct {
				Mappings []struct {
					URI  string `mapstructure:"uri" json:"uri" yaml:"uri"`
					Path string `mapstructure:"path" json:"path" yaml:"path"`
				} `mapstructure:"mappings" json:"mappings" yaml:"mappings"`
			} `mapstructure:"static" json:"static" yaml:"static"`
		} `mapstructure:"server" json:"server" yaml:"server"`
	}
	if err := server.config.Unmarshal(&values); err == nil {
		for _, mapping := range values.Server.Static.Mappings {
			if mapping.URI != "" && mapping.Path != "" {
				if err := server.StaticWithOptions(mapping.URI, mapping.Path, StaticOptions{
					CacheControl: configString(server.config.Get("server.static.cacheControl")),
					Index:        configString(server.config.Get("server.static.index")),
					SPAFallback:  configBoolValue(server.config.Get("server.static.spaFallback")),
					Extensions:   configStrings(server.config.Get("server.static.extensions")),
				}); err != nil {
					server.addInitializationError(fmt.Errorf("configure static mapping %s: %w", mapping.URI, err))
				}
			}
		}
	} else {
		server.addInitializationError(fmt.Errorf("configure static mappings: %w", err))
	}
}

func configString(value any) string {
	if value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func configBool(value any) (bool, bool) {
	if value == nil {
		return false, false
	}
	switch value := value.(type) {
	case bool:
		return value, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return parsed, err == nil
	default:
		return false, false
	}
}

func configInt(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case int32:
		return int64(value), true
	case float64:
		return int64(value), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func configDuration(value any) (time.Duration, bool) {
	if value == nil {
		return 0, false
	}
	if duration, ok := value.(time.Duration); ok {
		return duration, duration > 0
	}
	if text, ok := value.(string); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(text))
		return duration, err == nil && duration > 0
	}
	if number, ok := configInt(value); ok && number > 0 {
		return time.Duration(number), true
	}
	return 0, false
}

func (server *Server) registerValidatorNames() {
	register := func(validation *validator.Validate) {
		validation.RegisterTagNameFunc(func(field reflect.StructField) string {
			name := field.Tag.Get("json")
			if name == "" {
				name = field.Name
			} else if comma := strings.IndexByte(name, ','); comma >= 0 {
				name = name[:comma]
			}
			if name == "-" || name == "" {
				return field.Name
			}
			return name
		})
	}
	if !server.customBindingValidator {
		register(server.bindingValidator)
	}
	if !server.customValidateValidator {
		register(server.validateValidator)
	}
}
