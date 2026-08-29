package ghttp

import (
	"time"

	"github.com/lanechi/gonex/logging"
)

// Optional stores a configuration value together with whether it was
// explicitly supplied. It is used only during Server construction.
type optional[T any] struct {
	Value T
	Set   bool
}

// TLSOptions groups the TLS settings controlled by construction options.
type TLSOptions struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

// SessionOptions groups session manager and cookie settings.
type SessionOptions struct {
	Manager       *SessionManager
	CookieOptions *CookieOptions
}

// SecurityOptions groups host, CORS, and CSRF settings.
type SecurityOptions struct {
	AllowedHosts []string
	CORS         CORSOptions
	CSRF         CSRFOptions
}

// SchedulerOptions controls the built-in Server scheduler. A nil Location
// uses time.Local for cron expressions without an explicit timezone prefix.
type SchedulerOptions struct {
	Enabled  bool
	Location *time.Location
}

// serverOptions is the single source of explicit construction values. Runtime
// state remains owned by Server; configuration resolution reads these values
// to enforce a deterministic precedence order.
type serverOptions struct {
	Config          optional[Config]
	Logger          optional[logging.Logger]
	Address         optional[string]
	Mode            optional[string]
	ReadTimeout     optional[time.Duration]
	WriteTimeout    optional[time.Duration]
	IdleTimeout     optional[time.Duration]
	BodyLimit       optional[int64]
	MultipartLimit  optional[int64]
	HeaderLimit     optional[int]
	ShutdownTimeout optional[time.Duration]
	TLS             optional[TLSOptions]
	OpenAPI         optional[OpenAPIOptions]
	OpenAPIPath     optional[string]
	SwaggerPath     optional[string]
	Session         optional[*SessionManager]
	SessionCookie   optional[*CookieOptions]
	CORS            optional[CORSOptions]
	TemplateRoot    optional[string]
	AllowedHosts    optional[[]string]
	CSRF            optional[CSRFOptions]
	Scheduler       optional[SchedulerOptions]
}
