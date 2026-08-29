package ghttp

import (
	"time"

	"github.com/lanechi/gonex/logging"
)

// Optional stores a configuration value together with whether it was
// explicitly supplied. It is used only during Server construction.
type Optional[T any] struct {
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
	Config          Optional[Config]
	Logger          Optional[logging.Logger]
	Address         Optional[string]
	Mode            Optional[string]
	ReadTimeout     Optional[time.Duration]
	WriteTimeout    Optional[time.Duration]
	IdleTimeout     Optional[time.Duration]
	BodyLimit       Optional[int64]
	MultipartLimit  Optional[int64]
	HeaderLimit     Optional[int]
	ShutdownTimeout Optional[time.Duration]
	TLS             Optional[TLSOptions]
	OpenAPI         Optional[OpenAPIOptions]
	OpenAPIPath     Optional[string]
	SwaggerPath     Optional[string]
	Session         Optional[*SessionManager]
	SessionCookie   Optional[*CookieOptions]
	CORS            Optional[CORSOptions]
	TemplateRoot    Optional[string]
	AllowedHosts    Optional[[]string]
	CSRF            Optional[CSRFOptions]
	Scheduler       Optional[SchedulerOptions]
}
