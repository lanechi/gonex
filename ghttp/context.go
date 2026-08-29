package ghttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/session"
	"github.com/lanechi/gonex/template"
)

type contextKey struct{}

const ginFrameworkContextKey = "gonex.framework.context"

// Context is the framework context over the current Gin request.
type Context struct {
	gin             *gin.Context
	server          *Server
	sessionManager  *SessionManager
	templateManager *template.Manager
	logger          logging.Logger
	wroteResponse   bool
	sessionMu       sync.Mutex
	session         session.Session
}

func newContext(ginContext *gin.Context) *Context {
	if ginContext == nil {
		return &Context{}
	}
	if existing, ok := ginContext.Get(ginFrameworkContextKey); ok {
		if frameworkContext, ok := existing.(*Context); ok && frameworkContext != nil {
			return frameworkContext
		}
	}
	frameworkContext := &Context{gin: ginContext}
	ginContext.Set(ginFrameworkContextKey, frameworkContext)
	return frameworkContext
}

func frameworkContextMiddleware(server *Server) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		frameworkContext := newContext(ginContext)
		frameworkContext.server = server
		frameworkContext.sessionManager = server.sessionManager
		frameworkContext.templateManager = server.templates
		frameworkContext.logger = requestLoggerFromGin(server, ginContext)
		requestContext := context.WithValue(ginContext.Request.Context(), contextKey{}, frameworkContext)
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		ginContext.Next()
	}
}

// FromContext returns the framework context attached to a controller's
// context.Context. It returns nil when called outside a framework request.
func FromContext(ctx context.Context) *Context {
	if ctx == nil {
		return nil
	}
	value := ctx.Value(contextKey{})
	frameworkContext, _ := value.(*Context)
	return frameworkContext
}

// Gin returns the underlying Gin context for integrations that need it.
func (ctx *Context) Gin() *gin.Context {
	if ctx == nil {
		return nil
	}
	return ctx.gin
}

func (ctx *Context) Request() *http.Request {
	if ctx == nil || ctx.gin == nil {
		return nil
	}
	return ctx.gin.Request
}

// Response returns the current response writer for integrations that need a
// standard net/http writer.
func (ctx *Context) Response() http.ResponseWriter {
	if ctx == nil || ctx.gin == nil {
		return nil
	}
	return ctx.gin.Writer
}

// Logger returns the server logger associated with this request.
func (ctx *Context) Logger() logging.Logger {
	if ctx == nil {
		return nil
	}
	return ctx.logger
}

// ClientIP returns the client address resolved by Gin's trusted-proxy policy.
func (ctx *Context) ClientIP() string {
	if ctx == nil || ctx.gin == nil {
		return ""
	}
	return ctx.gin.ClientIP()
}

func (ctx *Context) Param(name string) string {
	if ctx == nil || ctx.gin == nil {
		return ""
	}
	return ctx.gin.Param(name)
}

func (ctx *Context) Query(name string) string {
	if ctx == nil || ctx.gin == nil {
		return ""
	}
	return ctx.gin.Query(name)
}

func (ctx *Context) Header(name string) string {
	if ctx == nil || ctx.gin == nil {
		return ""
	}
	return ctx.gin.GetHeader(name)
}

// RequestID returns the current request correlation ID.
func (ctx *Context) RequestID() string {
	if ctx == nil || ctx.gin == nil {
		return ""
	}
	return requestIDFromGin(ctx.gin)
}

func (ctx *Context) Set(key string, value any) {
	if ctx != nil && ctx.gin != nil {
		ctx.gin.Set(key, value)
	}
}

func (ctx *Context) Get(key string) (any, bool) {
	if ctx == nil || ctx.gin == nil {
		return nil, false
	}
	return ctx.gin.Get(key)
}

func (ctx *Context) JSON(status int, value any) {
	if ctx != nil && ctx.gin != nil {
		ctx.wroteResponse = true
		ctx.gin.JSON(status, value)
	}
}

// String writes a plain-text response.
func (ctx *Context) String(status int, format string, values ...any) {
	if ctx != nil && ctx.gin != nil {
		ctx.wroteResponse = true
		ctx.gin.String(status, format, values...)
	}
}

// File serves a local file through net/http.
func (ctx *Context) File(status int, path string) {
	if ctx == nil || ctx.gin == nil {
		return
	}
	ctx.wroteResponse = true
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		ctx.gin.Status(http.StatusNotFound)
		return
	}
	ctx.gin.Status(status)
	http.ServeFile(ctx.gin.Writer, ctx.gin.Request, path)
}

// Redirect redirects the current request.
func (ctx *Context) Redirect(status int, location string) {
	if ctx != nil && ctx.gin != nil {
		ctx.wroteResponse = true
		ctx.gin.Redirect(status, location)
	}
}

// Stream writes a streaming response until the callback returns false.
func (ctx *Context) Stream(status int, contentType string, step func(io.Writer) bool) {
	if ctx == nil || ctx.gin == nil {
		return
	}
	ctx.wroteResponse = true
	ctx.gin.Header("Content-Type", contentType)
	ctx.gin.Status(status)
	for {
		select {
		case <-ctx.gin.Request.Context().Done():
			return
		default:
		}
		if !step(ctx.gin.Writer) {
			return
		}
		if flusher, ok := ctx.gin.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

// Abort stops the current middleware chain.
func (ctx *Context) Abort() {
	if ctx != nil && ctx.gin != nil {
		ctx.gin.Abort()
	}
}

// HTML renders a named server template into the current response.
func (ctx *Context) HTML(status int, name string, data any) error {
	if ctx == nil || ctx.gin == nil || ctx.templateManager == nil {
		return fmt.Errorf("template manager is not configured")
	}
	var output bytes.Buffer
	if err := ctx.templateManager.Execute(&output, name, data); err != nil {
		return err
	}
	ctx.wroteResponse = true
	ctx.gin.Data(status, "text/html; charset=utf-8", output.Bytes())
	return nil
}

// Session opens the current request's session.
func (ctx *Context) Session() (session.Session, error) {
	if ctx == nil || ctx.sessionManager == nil {
		return nil, fmt.Errorf("session manager is not configured")
	}
	ctx.sessionMu.Lock()
	defer ctx.sessionMu.Unlock()
	if ctx.session != nil {
		return ctx.session, nil
	}
	current, err := ctx.sessionManager.Open(ctx)
	if err != nil {
		return nil, err
	}
	ctx.session = current
	return current, nil
}

func (ctx *Context) evictSession(current *managedSession) {
	if ctx == nil || current == nil {
		return
	}
	ctx.sessionMu.Lock()
	if ctx.session == current {
		ctx.session = nil
	}
	ctx.sessionMu.Unlock()
}
