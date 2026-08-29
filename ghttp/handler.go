package ghttp

import (
	"context"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/router"
)

// Response is the default success envelope used by the initial framework
// implementation. The encoder becomes replaceable in Phase 2.
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Details any    `json:"details,omitempty"`
}

func (server *Server) handlerFor(route router.Definition) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		runtime := route.Runtime
		requestType := route.Metadata.RequestType
		frameworkContext := newContext(ginContext)
		frameworkContext.server = server
		frameworkContext.sessionManager = server.sessionManager
		frameworkContext.templateManager = server.templates
		frameworkContext.logger = requestLoggerFromGin(server, ginContext)
		requestContext := context.WithValue(ginContext.Request.Context(), contextKey{}, frameworkContext)
		requestValue := reflect.New(requestType.Elem())
		if err := runtime.Binder.Bind(ginContext, requestValue.Interface()); err != nil {
			server.handleError(frameworkContext, frameworkError(err))
			return
		}
		if err := server.validateRequest(requestValue.Interface(), runtime.Binder.HasBindingRules(), runtime.Binder.HasValidateRules()); err != nil {
			server.handleError(frameworkContext, err)
			return
		}

		results := runtime.MethodValue.Call([]reflect.Value{
			reflect.ValueOf(requestContext),
			requestValue,
		})

		if len(results) == 1 {
			if err := errorFromValue(results[0]); err != nil {
				server.handleError(frameworkContext, err)
				return
			}
			if !frameworkContext.wroteResponse {
				server.encodeResponse(frameworkContext, nil)
			}
			return
		}

		if err := errorFromValue(results[1]); err != nil {
			server.handleError(frameworkContext, err)
			return
		}
		var data any
		if !isNilValue(results[0]) {
			data = results[0].Interface()
		}
		if !frameworkContext.wroteResponse {
			server.encodeResponse(frameworkContext, data)
		}
	}
}

func frameworkError(err error) error {
	if bindingErr, ok := err.(*router.BindingError); ok {
		return &Error{Code: bindingErr.Code, HTTPStatus: bindingErr.HTTPStatus, Message: bindingErr.Message, Cause: bindingErr.Cause}
	}
	return err
}

func errorFromValue(value reflect.Value) error {
	if !value.IsValid() || isNilValue(value) {
		return nil
	}
	err, _ := value.Interface().(error)
	return err
}

func isNilValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (server *Server) encodeResponse(ctx *Context, data any) {
	if err := server.responseEncoder.Encode(ctx, data); err != nil {
		server.handleError(ctx, err)
	}
}

func (server *Server) handleError(ctx *Context, err error) {
	if ctx != nil && ctx.gin != nil && err != nil {
		_ = ctx.gin.Error(err)
		// A controller may intentionally write a direct response and still
		// return an error for logging/metrics. Once headers are committed, an
		// additional error envelope would corrupt the response body.
		if ctx.gin.Writer.Written() {
			return
		}
	}
	server.errorHandler(ctx, err)
}
