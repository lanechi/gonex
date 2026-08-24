package ghttp_test

import (
	"context"
	"mime/multipart"
	"net/http"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

type helloRequest struct {
	g.Meta `path:"/hello" method:"get" tags:"Hello" summary:"Hello API"`
}

type helloResponse struct {
	Message string `json:"message"`
}

type helloController struct{}

func (*helloController) Hello(ctx context.Context, _ *helloRequest) (*helloResponse, error) {
	if ghttp.FromContext(ctx) == nil {
		return nil, ghttp.NewError(5001, http.StatusInternalServerError, "framework context is missing")
	}
	return &helloResponse{Message: "hello"}, nil
}

type emptyRequest struct {
	g.Meta `path:"/empty" method:"get"`
}

type emptyController struct{}

func (*emptyController) Empty(context.Context, *emptyRequest) error { return nil }

type bindingRequest struct {
	g.Meta  `       path:"/users/:id" method:"post"`
	ID      int64  `path:"id"                       binding:"required"`
	Page    int    `                                binding:"gte=1"    query:"page"`
	Token   string `                                binding:"required"              header:"Authorization"`
	Session string `                                                                                       cookie:"session_id"`
	Name    string `                                binding:"required"                                                         json:"name"`
	Age     int    `                                                                                                           json:"age"  validate:"gte=0"`
}

type bindingResponse struct {
	ID      int64  `json:"id"`
	Page    int    `json:"page"`
	Token   string `json:"token"`
	Session string `json:"session"`
	Name    string `json:"name"`
	Age     int    `json:"age"`
}

type bindingController struct{}

func (*bindingController) Create(_ context.Context, request *bindingRequest) (*bindingResponse, error) {
	return &bindingResponse{
		ID:      request.ID,
		Page:    request.Page,
		Token:   request.Token,
		Session: request.Session,
		Name:    request.Name,
		Age:     request.Age,
	}, nil
}

type formRequest struct {
	g.Meta `       path:"/form" method:"post"`
	Name   string `                           form:"name"  binding:"required"`
	Count  int    `                           form:"count" binding:"required"`
}

type formResponse struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type formController struct{}

func (*formController) Submit(_ context.Context, request *formRequest) (*formResponse, error) {
	return &formResponse{Name: request.Name, Count: request.Count}, nil
}

type panicRequest struct {
	g.Meta `path:"/panic" method:"get"`
}

type panicController struct{}

func (*panicController) Panic(context.Context, *panicRequest) (*helloResponse, error) {
	panic("test panic")
}

type recordingLogger struct {
	messages      []string
	fields        [][]logging.Field
	root          *recordingLogger
	contextFields []logging.Field
	contextName   string
}

func (logger *recordingLogger) Debug(_ context.Context, message string, fields ...logging.Field) {
	logger.record(message, fields)
}
func (logger *recordingLogger) Info(_ context.Context, message string, fields ...logging.Field) {
	logger.record(message, fields)
}
func (logger *recordingLogger) Warn(_ context.Context, message string, fields ...logging.Field) {
	logger.record(message, fields)
}
func (logger *recordingLogger) Error(_ context.Context, message string, fields ...logging.Field) {
	logger.record(message, fields)
}

func (logger *recordingLogger) With(fields ...logging.Field) logging.Logger {
	root := logger.rootLogger()
	combined := make([]logging.Field, 0, len(logger.contextFields)+len(fields))
	combined = append(combined, logger.contextFields...)
	combined = append(combined, fields...)
	return &recordingLogger{root: root, contextFields: combined, contextName: logger.contextName}
}

func (logger *recordingLogger) Named(name string) logging.Logger {
	if name == "" {
		return logger
	}
	if logger.contextName != "" {
		name = logger.contextName + "." + name
	}
	return &recordingLogger{
		root:          logger.rootLogger(),
		contextFields: append([]logging.Field(nil), logger.contextFields...),
		contextName:   name,
	}
}

func (logger *recordingLogger) record(message string, fields []logging.Field) {
	root := logger.rootLogger()
	combined := make([]logging.Field, 0, len(logger.contextFields)+len(fields)+1)
	if logger.contextName != "" {
		combined = append(combined, logging.String("logger", logger.contextName))
	}
	combined = append(combined, logger.contextFields...)
	combined = append(combined, fields...)
	root.messages = append(root.messages, message)
	root.fields = append(root.fields, combined)
}

func (logger *recordingLogger) rootLogger() *recordingLogger {
	if logger.root != nil {
		return logger.root
	}
	return logger
}

func (logger *recordingLogger) Enabled(logging.Level) bool { return true }
func (logger *recordingLogger) Sync() error                { return nil }

type sessionRequest struct {
	g.Meta `path:"/session" method:"get"`
}

type sessionResponse struct {
	Value string `json:"value"`
}

type sessionController struct{}

func (*sessionController) Read(ctx context.Context, _ *sessionRequest) (*sessionResponse, error) {
	frameworkContext := ghttp.FromContext(ctx)
	session, err := frameworkContext.Session()
	if err != nil {
		return nil, err
	}
	value, _ := session.Get("value")
	if value == nil {
		if err := session.Set("value", "stored"); err != nil {
			return nil, err
		}
		value = "new"
	}
	return &sessionResponse{Value: value.(string)}, nil
}

type pageRequest struct {
	g.Meta `path:"/page" method:"get"`
}

type pageController struct{}

func (*pageController) Page(ctx context.Context, _ *pageRequest) error {
	return ghttp.FromContext(ctx).HTML(http.StatusOK, "index.html", map[string]string{"Name": "Lane"})
}

type uploadRequest struct {
	g.Meta `path:"/upload" method:"post"`
	Title  string                `                             form:"title" binding:"required"`
	File   *multipart.FileHeader `                                          binding:"required" file:"file"`
}

type uploadResponse struct {
	Title    string `json:"title"`
	Filename string `json:"filename"`
}

type uploadController struct{}

func (*uploadController) Upload(_ context.Context, request *uploadRequest) (*uploadResponse, error) {
	return &uploadResponse{Title: request.Title, Filename: request.File.Filename}, nil
}

type textRequest struct {
	g.Meta `path:"/text" method:"get"`
}

type textController struct{}

func (*textController) Text(ctx context.Context, _ *textRequest) error {
	ghttp.FromContext(ctx).String(http.StatusOK, "hello %s", "world")
	return nil
}

type documentedRequest struct {
	g.Meta `       path:"/documented" method:"post" tags:"Users" summary:"Create user" description:"Create a user" operationId:"createUser" deprecated:"true" security:"BearerAuth:write" consumes:"application/json" produces:"application/json"`
	Name   string `                                                                                                                                                                                                                               json:"name" dc:"Display name" example:"Lane" default:"guest" enum:"guest,admin" binding:"required,min=3"`
}

type documentedResponse struct {
	ID int64 `json:"id"`
}

type documentedController struct{}

func (*documentedController) Create(context.Context, *documentedRequest) (*documentedResponse, error) {
	return &documentedResponse{ID: 1}, nil
}
