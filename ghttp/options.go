package ghttp

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/scheduler"
)

// Option configures a Server during construction.
type Option func(*Server)

// TimeoutOptions groups the HTTP and graceful-shutdown timeouts.
type TimeoutOptions struct {
	Read, Write, Idle, Shutdown time.Duration
}

// OpenAPIOptions groups the documentation endpoint settings.
type OpenAPIOptions struct {
	Enabled      bool
	DocumentPath string
	SwaggerPath  string
}

// WithTimeouts configures all supplied server timeouts as one subsystem
// option. Non-positive values leave the corresponding default unchanged.
func WithTimeouts(options TimeoutOptions) Option {
	return WithHTTPTimeoutsAndShutdown(options.Read, options.Write, options.Idle, options.Shutdown)
}

func WithHTTPTimeoutsAndShutdown(read, write, idle, shutdown time.Duration) Option {
	return func(server *Server) {
		WithHTTPTimeouts(read, write, idle)(server)
		WithShutdownTimeout(shutdown)(server)
	}
}

// WithTLSOptions configures TLS as one subsystem option.
func WithTLSOptions(options TLSOptions) Option {
	return func(server *Server) {
		server.tlsEnabled, server.tlsCertFile, server.tlsKeyFile = options.Enabled, options.CertFile, options.KeyFile
		server.options.TLS = optional[TLSOptions]{Value: options, Set: true}
	}
}

// WithSessionOptions configures session ownership and cookie flags together.
func WithSessionOptions(options SessionOptions) Option {
	return func(server *Server) {
		if options.Manager != nil {
			server.sessionManager = options.Manager
			server.options.Session = optional[*SessionManager]{Value: options.Manager, Set: true}
		}
		if options.CookieOptions != nil {
			copy := *options.CookieOptions
			server.sessionCookieOptions = &copy
			server.options.SessionCookie = optional[*CookieOptions]{Value: &copy, Set: true}
		}
	}
}

// WithSecurityOptions configures host, CORS, and CSRF policy together.
func WithSecurityOptions(options SecurityOptions) Option {
	return func(server *Server) {
		server.allowedHosts = append([]string(nil), options.AllowedHosts...)
		server.options.AllowedHosts = optional[[]string]{Value: append([]string(nil), options.AllowedHosts...), Set: true}
		server.corsOptions = nil
		if options.CORS.Enabled {
			copy := options.CORS
			server.corsOptions = &copy
		}
		server.options.CORS = optional[CORSOptions]{Value: options.CORS, Set: true}
		server.csrfOptions = nil
		if options.CSRF.Enabled {
			copy := options.CSRF
			server.csrfOptions = &copy
		}
		server.options.CSRF = optional[CSRFOptions]{Value: options.CSRF, Set: true}
	}
}

// WithName sets the logical name of the server.
func WithName(name string) Option {
	return func(server *Server) {
		if name = strings.TrimSpace(name); name != "" {
			server.name = name
		}
	}
}

// WithAddress sets the TCP address used by Run.
func WithAddress(address string) Option {
	return func(server *Server) {
		if address = strings.TrimSpace(address); address != "" {
			server.address = address
			server.options.Address = optional[string]{Value: address, Set: true}
		}
	}
}

// WithHTTPTimeouts configures server-side connection timeouts.
func WithHTTPTimeouts(read, write, idle time.Duration) Option {
	return func(server *Server) {
		if read > 0 {
			server.readTimeout = read
			server.options.ReadTimeout = optional[time.Duration]{Value: read, Set: true}
		}
		if write > 0 {
			server.writeTimeout = write
			server.options.WriteTimeout = optional[time.Duration]{Value: write, Set: true}
		}
		if idle > 0 {
			server.idleTimeout = idle
			server.options.IdleTimeout = optional[time.Duration]{Value: idle, Set: true}
		}
	}
}

// WithRequestLimits configures request body, multipart memory, and header
// limits. A non-positive value leaves the corresponding default unchanged.
func WithRequestLimits(bodyBytes, multipartMemory int64, headerBytes int) Option {
	return func(server *Server) {
		if bodyBytes > 0 {
			server.maxBodyBytes = bodyBytes
			server.options.BodyLimit = optional[int64]{Value: bodyBytes, Set: true}
		}
		if multipartMemory > 0 {
			server.maxMultipartMemory = multipartMemory
			server.options.MultipartLimit = optional[int64]{Value: multipartMemory, Set: true}
		}
		if headerBytes > 0 {
			server.maxHeaderBytes = headerBytes
			server.options.HeaderLimit = optional[int]{Value: headerBytes, Set: true}
		}
	}
}

// WithRequestID controls automatic request ID generation and propagation.
// It is enabled by default; disable it for internal high-throughput paths that
// provide correlation IDs through another mechanism.
func WithRequestID(enabled bool) Option {
	return func(server *Server) {
		server.requestIDEnabled = enabled
	}
}

// WithEngine supplies a Gin engine. It is primarily useful for integrating
// an existing Gin setup while keeping the framework's route registry.
func WithEngine(engine *gin.Engine) Option {
	return func(server *Server) {
		if engine != nil {
			server.engine = engine
		}
	}
}

// WithLogger replaces the server logger.
func WithLogger(logger logging.Logger) Option {
	return func(server *Server) {
		if logger != nil {
			server.logger = logger
			server.options.Logger = optional[logging.Logger]{Value: logger, Set: true}
		}
	}
}

// WithScheduler supplies a scheduler whose lifecycle is managed exclusively by
// the configured Server. The caller must not inject the same instance into
// multiple Servers.
func WithScheduler(manager scheduler.Scheduler) Option {
	return func(server *Server) {
		if isNilInterface(manager) {
			server.addInitializationError(fmt.Errorf("scheduler must not be nil"))
			return
		}
		server.scheduler = manager
		server.schedulerEnabled = true
		server.options.Scheduler = optional[SchedulerOptions]{Value: SchedulerOptions{Enabled: true}, Set: true}
	}
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflectValue.IsNil()
	default:
		return false
	}
}

// WithSchedulerOptions configures the Server-owned default scheduler. An
// explicitly supplied scheduler through WithScheduler retains its own engine
// configuration and lifecycle ownership remains with Server.
func WithSchedulerOptions(options SchedulerOptions) Option {
	return func(server *Server) {
		server.schedulerEnabled = options.Enabled
		server.schedulerLocation = options.Location
		server.options.Scheduler = optional[SchedulerOptions]{Value: options, Set: true}
	}
}

// WithConfig attaches an application configuration object to the server.
func WithConfig(configuration Config) Option {
	return func(server *Server) {
		if configuration != nil {
			server.config = configuration
			server.options.Config = optional[Config]{Value: configuration, Set: true}
		}
	}
}

// WithSessionManager attaches a session manager to the server.
func WithSessionManager(manager *SessionManager) Option {
	return func(server *Server) {
		if manager != nil {
			server.sessionManager = manager
			server.options.Session = optional[*SessionManager]{Value: manager, Set: true}
		}
	}
}

// WithSessionCookieOptions configures the session identifier cookie flags.
func WithSessionCookieOptions(options CookieOptions) Option {
	return func(server *Server) {
		copy := options
		server.sessionCookieOptions = &copy
		server.options.SessionCookie = optional[*CookieOptions]{Value: &copy, Set: true}
	}
}

// WithCORS enables CORS middleware during construction.
func WithCORS(configuration CORSOptions) Option {
	return func(server *Server) {
		server.options.CORS = optional[CORSOptions]{Value: configuration, Set: true}
		if configuration.Enabled {
			server.corsOptions = &configuration
		} else {
			server.corsOptions = nil
		}
	}
}

// WithResponseEncoder replaces the default success response encoder.
func WithResponseEncoder(encoder ResponseEncoder) Option {
	return func(server *Server) {
		if encoder != nil {
			server.responseEncoder = encoder
		}
	}
}

// WithErrorHandler replaces the default controller, binding, and validation
// error handler.
func WithErrorHandler(handler ErrorHandler) Option {
	return func(server *Server) {
		if handler != nil {
			server.errorHandler = handler
		}
	}
}

// WithValidator replaces the validator used for validate tags and custom
// struct-level rules. The caller must finish configuring validation before
// constructing the Server and must not mutate it afterward.
func WithValidator(validation *validator.Validate) Option {
	return func(server *Server) {
		if validation != nil {
			server.validateValidator = validation
			server.customValidateValidator = true
		}
	}
}

// WithBindingValidator replaces the validator used for binding tags. The
// supplied validator must already use "binding" as its tag name. It must be
// a different instance from the validator passed to WithValidator.
func WithBindingValidator(validation *validator.Validate) Option {
	return func(server *Server) {
		if validation != nil {
			server.bindingValidator = validation
			server.customBindingValidator = true
		}
	}
}

// WithOpenAPI configures interface documentation and both endpoint paths.
func WithOpenAPI(options OpenAPIOptions) Option {
	return func(server *Server) {
		server.openapiEnabled = options.Enabled
		server.options.OpenAPI = optional[OpenAPIOptions]{Value: options, Set: true}
		if options.DocumentPath != "" {
			server.openapiPath = options.DocumentPath
			server.options.OpenAPIPath = optional[string]{Value: options.DocumentPath, Set: true}
		}
		if options.SwaggerPath != "" {
			server.swaggerPath = options.SwaggerPath
			server.options.SwaggerPath = optional[string]{Value: options.SwaggerPath, Set: true}
		}
	}
}

// WithShutdownTimeout configures the maximum graceful shutdown duration.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(server *Server) {
		if timeout > 0 {
			server.shutdownTimeout = timeout
			server.options.ShutdownTimeout = optional[time.Duration]{Value: timeout, Set: true}
		}
	}
}

// WithTLS enables TLS when Run is used.
func WithTLS(certFile, keyFile string) Option {
	return func(server *Server) {
		server.tlsCertFile = certFile
		server.tlsKeyFile = keyFile
		server.tlsEnabled = certFile != "" || keyFile != ""
		server.options.TLS = optional[TLSOptions]{Value: TLSOptions{Enabled: server.tlsEnabled, CertFile: certFile, KeyFile: keyFile}, Set: true}
	}
}

// WithOpenAPIPath changes the generated OpenAPI document endpoint.
func WithOpenAPIPath(path string) Option {
	return func(server *Server) {
		if path != "" {
			server.openapiPath = path
			server.options.OpenAPIPath = optional[string]{Value: path, Set: true}
		}
	}
}

// WithSwaggerPath changes the embedded API explorer endpoint.
func WithSwaggerPath(path string) Option {
	return func(server *Server) {
		if path != "" {
			server.swaggerPath = path
			server.options.SwaggerPath = optional[string]{Value: path, Set: true}
		}
	}
}

// WithTemplateRoot loads templates during server construction.
func WithTemplateRoot(root string) Option {
	return func(server *Server) {
		if root != "" {
			server.options.TemplateRoot = optional[string]{Value: root, Set: true}
			if err := server.templates.SetRoot(root); err != nil {
				server.addInitializationError(fmt.Errorf("configure template root: %w", err))
			}
		}
	}
}

// WithAllowedHosts limits accepted Host headers.
func WithAllowedHosts(hosts ...string) Option {
	return func(server *Server) {
		server.options.AllowedHosts = optional[[]string]{Value: append([]string(nil), hosts...), Set: true}
		server.allowedHosts = append([]string(nil), hosts...)
	}
}

// WithCSRF enables double-submit-cookie CSRF protection.
func WithCSRF(options CSRFOptions) Option {
	return func(server *Server) {
		server.options.CSRF = optional[CSRFOptions]{Value: options, Set: true}
		if options.Enabled {
			server.csrfOptions = &options
		} else {
			server.csrfOptions = nil
		}
	}
}
