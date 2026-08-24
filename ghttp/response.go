package ghttp

import (
	"errors"
	"net/http"
)

// ResponseEncoder converts successful controller data into an HTTP response.
type ResponseEncoder interface {
	Encode(ctx *Context, data any) error
}

// ErrorHandler converts framework, binding, validation, and controller errors
// into an HTTP response.
type ErrorHandler func(ctx *Context, err error)

// DefaultResponseEncoder writes the framework's default response envelope.
type DefaultResponseEncoder struct{}

func (DefaultResponseEncoder) Encode(ctx *Context, data any) error {
	if data == nil {
		data = struct{}{}
	}
	ctx.JSON(200, Response{Code: 0, Message: "OK", Data: data})
	return nil
}

// defaultErrorHandler 使用公共 Response 结构统一返回框架错误。
// Response 定义在 handler.go 中，错误处理逻辑仍归 response.go 管理。
func defaultErrorHandler(ctx *Context, err error) {
	if ctx == nil || ctx.gin == nil || ctx.gin.Writer.Written() {
		return
	}
	status := http.StatusInternalServerError
	code := status
	message := "internal server error"
	var details any
	var frameworkError *Error
	if errors.As(err, &frameworkError) {
		status = frameworkError.HTTPStatus
		code = frameworkError.Code
		message = frameworkError.Message

		// details 可能包含字段名、校验规则或底层错误信息，只在 debug
		// 模式返回，避免 release/test 模式暴露过多实现细节。
		if ctx.server != nil && ctx.server.IsDebug() {
			details = frameworkError.Details
		}
	}
	if status < 100 || status > 999 {
		status = http.StatusInternalServerError
	}
	ctx.JSON(status, Response{Code: code, Message: message, Details: details})
}
