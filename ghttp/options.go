package ghttp

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/lanechi/gonex/logging"
)

// Option configures a Server during construction.
type Option func(*Server)

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
			server.addressSet = true
		}
	}
}

// WithHTTPTimeouts configures server-side connection timeouts.
func WithHTTPTimeouts(read, write, idle time.Duration) Option {
	return func(server *Server) {
		if read > 0 {
			server.readTimeout = read
			server.readTimeoutSet = true
		}
		if write > 0 {
			server.writeTimeout = write
			server.writeTimeoutSet = true
		}
		if idle > 0 {
			server.idleTimeout = idle
			server.idleTimeoutSet = true
		}
	}
}

// WithRequestLimits configures request body, multipart memory, and header
// limits. A non-positive value leaves the corresponding default unchanged.
func WithRequestLimits(bodyBytes, multipartMemory int64, headerBytes int) Option {
	return func(server *Server) {
		if bodyBytes > 0 {
			server.maxBodyBytes = bodyBytes
			server.bodyLimitSet = true
		}
		if multipartMemory > 0 {
			server.maxMultipartMemory = multipartMemory
			server.multipartLimitSet = true
		}
		if headerBytes > 0 {
			server.maxHeaderBytes = headerBytes
			server.headerLimitSet = true
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
			server.loggerSet = true
		}
	}
}

// WithConfig attaches an application configuration object to the server.
func WithConfig(configuration Config) Option {
	return func(server *Server) {
		if configuration != nil {
			server.config = configuration
			server.configSet = true
		}
	}
}

// WithSessionManager attaches a session manager to the server.
func WithSessionManager(manager *SessionManager) Option {
	return func(server *Server) {
		if manager != nil {
			server.sessionManager = manager
			server.sessionSet = true
		}
	}
}

// WithSessionCookieOptions configures the session identifier cookie flags.
func WithSessionCookieOptions(options CookieOptions) Option {
	return func(server *Server) {
		copy := options
		server.sessionCookieOptions = &copy
	}
}

// WithCORS enables CORS middleware during construction.
func WithCORS(configuration CORSOptions) Option {
	return func(server *Server) {
		server.corsSet = true
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

// WithOpenAPI controls the interface documentation feature at construction
// time, including both the generated OpenAPI JSON and Swagger UI endpoints.
func WithOpenAPI(enabled bool) Option {
	return func(server *Server) {
		server.openapiEnabled = enabled
		server.openapiSet = true
	}
}

// WithShutdownTimeout configures the maximum graceful shutdown duration.
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(server *Server) {
		if timeout > 0 {
			server.shutdownTimeout = timeout
			server.shutdownSet = true
		}
	}
}

// WithTLS enables TLS when Run is used.
func WithTLS(certFile, keyFile string) Option {
	return func(server *Server) {
		server.tlsCertFile = certFile
		server.tlsKeyFile = keyFile
		server.tlsEnabled = certFile != "" || keyFile != ""
		server.tlsSet = true
	}
}

// WithOpenAPIPath changes the generated OpenAPI document endpoint.
func WithOpenAPIPath(path string) Option {
	return func(server *Server) {
		if path != "" {
			server.openapiPath = path
			server.openapiPathSet = true
		}
	}
}

// WithSwaggerPath changes the embedded API explorer endpoint.
func WithSwaggerPath(path string) Option {
	return func(server *Server) {
		if path != "" {
			server.swaggerPath = path
			server.swaggerPathSet = true
		}
	}
}

// WithTemplateRoot loads templates during server construction.
func WithTemplateRoot(root string) Option {
	return func(server *Server) {
		if root != "" {
			server.templateRootSet = true
			if err := server.templates.SetRoot(root); err != nil {
				server.addInitializationError(fmt.Errorf("configure template root: %w", err))
			}
		}
	}
}

// WithAllowedHosts limits accepted Host headers.
func WithAllowedHosts(hosts ...string) Option {
	return func(server *Server) {
		server.allowedHostsSet = true
		server.allowedHosts = append([]string(nil), hosts...)
	}
}

// WithCSRF enables double-submit-cookie CSRF protection.
func WithCSRF(options CSRFOptions) Option {
	return func(server *Server) {
		server.csrfSet = true
		if options.Enabled {
			server.csrfOptions = &options
		} else {
			server.csrfOptions = nil
		}
	}
}
