package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestIDHeader = "X-Request-ID"
const maxRequestIDLength = 128

func RequestID() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := strings.TrimSpace(context.GetHeader(RequestIDHeader))
		if !validRequestID(requestID) {
			requestID = NewRequestID()
		}
		context.Set(RequestIDHeader, requestID)
		context.Header(RequestIDHeader, requestID)
		context.Next()
	}
}

func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(context *gin.Context) {
		if maxBytes > 0 && context.Request.ContentLength > maxBytes {
			context.Abort()
			context.JSON(http.StatusRequestEntityTooLarge, map[string]any{"code": 41300, "message": "request body is too large"})
			return
		}
		if maxBytes > 0 && context.Request.Body != nil {
			context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxBytes)
		}
		context.Next()
	}
}

func validRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDLength {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func HostValidation(allowed []string) gin.HandlerFunc {
	return func(context *gin.Context) {
		if len(allowed) == 0 || HostAllowed(context.Request.Host, allowed) {
			context.Next()
			return
		}
		context.Abort()
		context.JSON(http.StatusMisdirectedRequest, map[string]any{"code": 42100, "message": "invalid host"})
	}
}

func HostAllowed(host string, allowed []string) bool {
	host = normalizeHost(host)
	for _, candidate := range allowed {
		candidate = normalizeHost(candidate)
		wildcardSuffix := strings.TrimPrefix(candidate, "*")
		if host == candidate || strings.HasPrefix(candidate, "*.") && len(host) > len(wildcardSuffix) && strings.HasSuffix(host, wildcardSuffix) {
			return true
		}
	}
	return false
}

func normalizeHost(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Host
		}
	}
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		value = parsedHost
	}
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	return strings.TrimSuffix(value, ".")
}

func CSRF(options CSRFOptions) gin.HandlerFunc {
	if options.CookieName == "" {
		options.CookieName = "csrf_token"
	}
	if options.HeaderName == "" {
		options.HeaderName = "X-CSRF-Token"
	}
	if options.SameSite == http.SameSiteDefaultMode {
		options.SameSite = http.SameSiteLaxMode
	}
	return func(context *gin.Context) {
		cookie, err := context.Request.Cookie(options.CookieName)
		if err != nil || cookie.Value == "" {
			cookieValue, tokenErr := NewSecureToken()
			if tokenErr != nil {
				context.Abort()
				context.JSON(http.StatusInternalServerError, map[string]any{"code": 50000, "message": "internal server error"})
				return
			}
			http.SetCookie(context.Writer, &http.Cookie{
				Name: options.CookieName, Value: cookieValue, Path: "/", Domain: options.Domain,
				Secure: options.Secure, SameSite: options.SameSite,
			})
			cookie = &http.Cookie{Value: cookieValue}
		}
		if context.Request.Method == http.MethodGet || context.Request.Method == http.MethodHead || context.Request.Method == http.MethodOptions || context.Request.Method == http.MethodTrace {
			context.Next()
			return
		}
		headerValue := context.GetHeader(options.HeaderName)
		if headerValue == "" || subtle.ConstantTimeCompare([]byte(headerValue), []byte(cookie.Value)) != 1 {
			context.Abort()
			context.JSON(http.StatusForbidden, map[string]any{"code": 40300, "message": "CSRF token is invalid"})
			return
		}
		context.Next()
	}
}

func NewRequestID() string {
	requestID, err := NewSecureToken()
	if err == nil {
		return requestID
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// NewSecureToken returns a cryptographically random 128-bit token.
func NewSecureToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func RequestIDFromGin(context *gin.Context) string {
	if value, exists := context.Get(RequestIDHeader); exists {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return context.GetHeader(RequestIDHeader)
}
