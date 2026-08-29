package ghttp

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/session"
)

// Config is the configuration contract used by Server.
type Config interface {
	Get(key string) any
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	Unmarshal(target any) error
}

func (server *Server) applyLoggerConfig() {
	if server.config == nil {
		return
	}
	get := server.config.Get
	if enabled, ok := firstConfigBool(get,
		"logger.enabled", "logger.show", "server.log.enabled", "server.log.show",
	); ok && !enabled {
		server.logger = logging.NewNopLogger()
	} else if !server.options.Logger.Set {
		if logger, err := logging.NewConfiguredLoggerFromConfig(server.config); err != nil {
			server.addInitializationError(fmt.Errorf("configure logger: %w", err))
		} else if logger != nil {
			server.logger = logger
		}
	}
}

func (server *Server) applyConfig() {
	if server.config == nil {
		return
	}
	get := server.config.Get
	if !server.options.Address.Set {
		if value := configString(get("server.address")); value != "" && value != "<nil>" {
			server.address = value
		}
	}
	if !server.options.ReadTimeout.Set {
		if value, ok := configDuration(get("server.readTimeout")); ok {
			server.readTimeout = value
		}
	}
	if !server.options.WriteTimeout.Set {
		if value, ok := configDuration(get("server.writeTimeout")); ok {
			server.writeTimeout = value
		}
	}
	if !server.options.IdleTimeout.Set {
		if value, ok := configDuration(get("server.idleTimeout")); ok {
			server.idleTimeout = value
		}
	}
	if !server.options.BodyLimit.Set {
		if value, ok := configInt(get("server.maxBodyBytes")); ok && value > 0 {
			server.maxBodyBytes = value
		}
	}
	if !server.options.MultipartLimit.Set {
		if value, ok := configInt(get("server.maxMultipartMemory")); ok && value > 0 {
			server.maxMultipartMemory = value
		}
	}
	if !server.options.HeaderLimit.Set {
		if value, ok := configInt(get("server.maxHeaderBytes")); ok && value > 0 && value <= math.MaxInt {
			server.maxHeaderBytes = int(value)
		} else if ok && value > math.MaxInt {
			server.addInitializationError(fmt.Errorf("server.maxHeaderBytes exceeds platform int range"))
		}
	}
	if !server.options.ShutdownTimeout.Set {
		if raw := get("server.shutdownTimeout"); raw != nil {
			if value, ok := configDuration(raw); !ok {
				server.addInitializationError(fmt.Errorf("server.shutdownTimeout must be a valid duration greater than zero"))
			} else {
				server.shutdownTimeout = value
			}
		}
	}
	if !server.options.Scheduler.Set {
		if enabled, ok := configBool(get("server.scheduler.enabled")); ok {
			server.schedulerEnabled = enabled
		}
		if timezone := configString(get("server.scheduler.timezone")); timezone != "" && timezone != "<nil>" {
			location, err := time.LoadLocation(timezone)
			if err != nil {
				server.addInitializationError(fmt.Errorf("configure scheduler timezone: %w", err))
			} else {
				server.schedulerLocation = location
			}
		}
	}
	if !server.options.TLS.Set {
		server.tlsCertFile = configString(get("server.tls.cert"))
		server.tlsKeyFile = configString(get("server.tls.key"))
		if enabled, ok := configBool(get("server.tls.enabled")); ok {
			server.tlsEnabled = enabled
		} else {
			server.tlsEnabled = server.tlsCertFile != "" && server.tlsKeyFile != ""
		}
	}
	if !server.options.OpenAPI.Set {
		if enabled, ok := configBool(get("server.openapi.enabled")); ok {
			server.openapiEnabled = enabled
		}
	}
	if !server.options.OpenAPIPath.Set {
		if path := configString(get("server.openapi.path")); path != "" && path != "<nil>" {
			server.openapiPath = path
		}
	}
	if !server.options.SwaggerPath.Set {
		if path := configString(get("server.swagger.path")); path != "" && path != "<nil>" {
			server.swaggerPath = path
		}
	}
	if !server.options.TemplateRoot.Set {
		if root := configString(get("server.template.root")); root != "" && root != "<nil>" {
			if err := server.templates.SetRoot(root); err != nil {
				server.addInitializationError(fmt.Errorf("configure template root: %w", err))
			}
		}
	}
	if !server.options.CORS.Set {
		if enabled, ok := configBool(get("cors.enabled")); ok && enabled {
			options := CORSOptions{
				Enabled:          true,
				AllowOrigins:     configStrings(get("cors.allowOrigins")),
				AllowMethods:     configStrings(get("cors.allowMethods")),
				AllowHeaders:     configStrings(get("cors.allowHeaders")),
				ExposeHeaders:    configStrings(get("cors.exposeHeaders")),
				AllowCredentials: configBoolValue(get("cors.allowCredentials")),
			}
			if maxAge, ok := configInt(get("cors.maxAge")); ok {
				options.MaxAge = int(maxAge)
			}
			server.corsOptions = &options
		}
	}
	if !server.options.Session.Set {
		if enabled, ok := configBool(get("session.enabled")); ok && !enabled {
			server.sessionManager = nil
		} else {
			name := configString(get("session.name"))
			ttl := server.sessionManager.ttl
			if value, ok := configDuration(get("session.ttl")); ok {
				ttl = value
			}
			storage := server.sessionManager.storage
			storageType := strings.ToLower(configString(get("session.storage.type")))
			switch storageType {
			case "", "memory":
			case "cookie":
				secretValue := configString(get("session.storage.secret"))
				if secretValue == "" || secretValue == "<nil>" {
					secretValue = configString(get("session.secret"))
				}
				revocationType := strings.ToLower(strings.TrimSpace(configString(get("session.storage.revocation"))))
				if revocationType != "memory" {
					if revocationType == "" || revocationType == "<nil>" {
						server.addInitializationError(fmt.Errorf("configure cookie session storage: session.storage.revocation must be explicitly set to memory, or provide a shared CookieRevocationStore through WithSessionManager"))
					} else {
						server.addInitializationError(fmt.Errorf("unsupported session.storage.revocation %q", revocationType))
					}
					break
				}
				cookieStorage, err := session.NewCookieStorage([]byte(secretValue), session.NewMemoryCookieRevocationStore())
				if err != nil {
					server.addInitializationError(fmt.Errorf("configure cookie session storage: %w", err))
				} else {
					storage = cookieStorage
				}
			default:
				server.addInitializationError(fmt.Errorf("unsupported session.storage.type %q", storageType))
			}
			if name != "" && name != "<nil>" || ttl != 24*time.Hour || storage != server.sessionManager.storage {
				server.sessionManager = NewSessionManager(storage, name, ttl)
			}
			if server.sessionManager != nil {
				options := server.sessionManager.cookieOptions
				if value := configString(get("session.path")); value != "" && value != "<nil>" {
					options.Path = value
				}
				if value := configString(get("session.domain")); value != "" && value != "<nil>" {
					options.Domain = value
				}
				if value, ok := configInt(get("session.maxAge")); ok {
					options.MaxAge = int(value)
				}
				if value, ok := configBool(get("session.secure")); ok {
					options.Secure = value
				}
				if value, ok := configBool(get("session.httpOnly")); ok {
					options.HTTPOnly = value
				}
				if value := configString(get("session.sameSite")); value != "" && value != "<nil>" {
					if sameSite, err := parseSameSite(value); err != nil {
						server.addInitializationError(err)
					} else {
						options.SameSite = sameSite
					}
				}
				server.sessionManager.cookieOptions = options
			}
		}
	}
	if proxies := configStrings(get("server.trustedProxies")); len(proxies) > 0 {
		if err := server.engine.SetTrustedProxies(proxies); err != nil {
			server.addInitializationError(fmt.Errorf("configure trusted proxies: %w", err))
		}
	}
	if !server.options.AllowedHosts.Set {
		server.allowedHosts = configStrings(get("server.allowedHosts"))
	}
	if !server.options.CSRF.Set {
		if enabled, ok := configBool(get("csrf.enabled")); ok && enabled {
			options := CSRFOptions{
				Enabled:    true,
				CookieName: configString(get("csrf.cookieName")),
				HeaderName: configString(get("csrf.headerName")),
				Domain:     configString(get("csrf.domain")),
				Secure:     configBoolValue(get("csrf.secure")),
			}
			if sameSite, err := parseSameSite(configString(get("csrf.sameSite"))); err != nil {
				server.addInitializationError(err)
			} else {
				options.SameSite = sameSite
			}
			server.csrfOptions = &options
		}
	}
}

func firstConfigBool(get func(string) any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := configBool(get(key)); ok {
			return value, true
		}
	}
	return false, false
}

func configBoolValue(value any) bool {
	parsed, _ := configBool(value)
	return parsed
}

func configIntValue(value any) int64 {
	parsed, _ := configInt(value)
	return parsed
}

func configStrings(value any) []string {
	switch value := value.(type) {
	case []string:
		return append([]string{}, value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text := configString(item); text != "" && text != "<nil>" {
				result = append(result, text)
			}
		}
		return result
	case string:
		parts := strings.FieldsFunc(value, func(character rune) bool {
			return character == ',' || character == ' ' || character == '\t' || character == '\n'
		})
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
		return result
	default:
		return nil
	}
}

func parseSameSite(value string) (http.SameSite, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default", "lax":
		return http.SameSiteLaxMode, nil
	case "strict":
		return http.SameSiteStrictMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteDefaultMode, fmt.Errorf("invalid SameSite value %q", value)
	}
}
