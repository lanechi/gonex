package ghttp

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/middleware"
)

func toMiddlewareCSRFOptions(options CSRFOptions) middleware.CSRFOptions {
	return middleware.CSRFOptions{
		Enabled: options.Enabled, CookieName: options.CookieName, HeaderName: options.HeaderName,
		Domain: options.Domain, Secure: options.Secure, SameSite: options.SameSite,
	}
}

const requestIDHeader = middleware.RequestIDHeader

func requestIDMiddleware(server *Server) gin.HandlerFunc {
	if server == nil || !server.requestIDEnabled {
		return func(context *gin.Context) { context.Next() }
	}
	return middleware.RequestID()
}

func requestLoggerMiddleware(server *Server) gin.HandlerFunc {
	return func(context *gin.Context) {
		logger := server.logger.Named("http")
		if requestID := requestIDFromGin(context); requestID != "" {
			logger = logger.With(logging.String("request_id", requestID))
		}
		request := context.Request.WithContext(logging.NewContext(context.Request.Context(), logger))
		context.Request = request
		context.Next()
	}
}

func frameworkFailureHandler(server *Server) middleware.FailureHandler {
	return func(context *gin.Context, status, code int, message string) {
		frameworkContext := newContext(context)
		frameworkContext.server = server
		frameworkContext.logger = requestLoggerFromGin(server, context)
		server.handleError(frameworkContext, NewError(code, status, message))
	}
}

func requestBodyLimitMiddleware(server *Server) gin.HandlerFunc {
	return middleware.BodyLimit(server.maxBodyBytes, frameworkFailureHandler(server))
}

func hostValidationMiddleware(server *Server) gin.HandlerFunc {
	failure := frameworkFailureHandler(server)
	return func(context *gin.Context) {
		server.settingsMu.RLock()
		allowed := append([]string(nil), server.allowedHosts...)
		server.settingsMu.RUnlock()
		if len(allowed) == 0 || middleware.HostAllowed(context.Request.Host, allowed) {
			context.Next()
			return
		}
		context.Abort()
		failure(context, http.StatusMisdirectedRequest, 42100, "invalid host")
	}
}

func csrfMiddleware(server *Server) gin.HandlerFunc {
	return func(context *gin.Context) {
		server.settingsMu.RLock()
		handler := server.csrfHandler
		server.settingsMu.RUnlock()
		if handler == nil {
			context.Next()
			return
		}
		handler(context)
	}
}

func recoveryMiddleware(server *Server) gin.HandlerFunc {
	return func(context *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger := requestLoggerFromGin(server, context)
				logger.Error(
					context.Request.Context(),
					"panic recovered",
					logging.Any("panic", recovered),
					logging.String("stack", string(debug.Stack())),
					logging.String("method", context.Request.Method),
					logging.String("path", context.Request.URL.Path),
				)
				context.Abort()
				if context.Writer.Written() {
					return
				}
				frameworkContext := newContext(context)
				frameworkContext.server = server
				frameworkContext.logger = logger
				server.handleError(frameworkContext, NewError(http.StatusInternalServerError, http.StatusInternalServerError, "internal server error"))
			}
		}()
		context.Next()
	}
}

func accessLogMiddleware(server *Server) gin.HandlerFunc {
	return func(context *gin.Context) {
		logger := requestLoggerFromGin(server, context)
		if !logger.Enabled(logging.InfoLevel) && !logger.Enabled(logging.WarnLevel) && !logger.Enabled(logging.ErrorLevel) {
			context.Next()
			return
		}
		started := time.Now()
		context.Next()
		request := context.Request
		requestSize := request.ContentLength
		if requestSize < 0 {
			requestSize = 0
		}
		responseSize := int64(context.Writer.Size())
		if responseSize < 0 {
			responseSize = 0
		}
		status := context.Writer.Status()
		fields := []logging.Field{
			logging.String("method", request.Method),
			logging.String("path", request.URL.Path),
			logging.Int("status", status),
			logging.Duration("duration", time.Since(started)),
			logging.String("client_ip", context.ClientIP()),
			logging.String("route", context.FullPath()),
			logging.Int64("request_size", requestSize),
			logging.Int64("response_size", responseSize),
		}
		if err := accessLogError(context); err != "" {
			fields = append(fields, logging.String("error", err))
		}
		switch {
		case status >= http.StatusInternalServerError:
			logger.Error(context.Request.Context(), "request completed", fields...)
		case status >= http.StatusBadRequest:
			logger.Warn(context.Request.Context(), "request completed", fields...)
		default:
			logger.Info(context.Request.Context(), "request completed", fields...)
		}
	}
}

func accessLogError(context *gin.Context) string {
	if context == nil || len(context.Errors) == 0 {
		return ""
	}
	return context.Errors.Last().Error()
}

func requestIDFromGin(context *gin.Context) string {
	return middleware.RequestIDFromGin(context)
}

func requestLoggerFromGin(server *Server, context *gin.Context) logging.Logger {
	if context != nil && context.Request != nil {
		if logger := logging.FromContext(context.Request.Context()); logger != nil {
			return logger
		}
	}
	if server != nil && server.logger != nil {
		return server.logger.Named("http")
	}
	return logging.NewNopLogger()
}
