// Package middleware contains middleware contracts and built-in option types.
package middleware

import (
	"net/http"
)

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
