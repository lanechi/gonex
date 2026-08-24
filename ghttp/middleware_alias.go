package ghttp

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Middleware is the application middleware contract.
type Middleware func(*gin.Context)

// CSRFOptions configures double-submit-cookie protection.
type CSRFOptions struct {
	Enabled    bool
	CookieName string
	HeaderName string
	Domain     string
	Secure     bool
	SameSite   http.SameSite
}

func ginMiddlewareHandlers(values []Middleware) []gin.HandlerFunc {
	result := make([]gin.HandlerFunc, len(values))
	for index, value := range values {
		result[index] = gin.HandlerFunc(value)
	}
	return result
}

func validateMiddleware(values []Middleware) error {
	for index, handler := range values {
		if handler == nil {
			return fmt.Errorf("middleware at index %d is nil", index)
		}
	}
	return nil
}
