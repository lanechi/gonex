// Package middleware contains middleware contracts and built-in option types.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FailureHandler converts a middleware failure into an HTTP response. Built-in
// middleware falls back to its standard JSON envelope when no handler is
// supplied, preserving standalone middleware behavior outside ghttp.Server.
type FailureHandler func(context *gin.Context, status, code int, message string)

// CORSOptions controls cross-origin requests.
type CORSOptions struct {
	Enabled          bool
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// CSRFOptions configures double-submit-cookie protection.
type CSRFOptions struct {
	Enabled    bool
	CookieName string
	HeaderName string
	Domain     string
	Secure     bool
	SameSite   http.SameSite
}
