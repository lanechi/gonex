// Package router
//
// Binder 使用示例：
//
//	type GetOneTestctrl2Req struct {
//		g.Meta `path:"/truetest/:id" method:"get"`
//		ID     int64 `path:"id" binding:"required" dc:"测试的"`
//	}
//
//	binder, err := router.NewBinder(reflect.TypeOf((*GetOneTestctrl2Req)(nil)))
//	if err != nil {
//		// 请求结构体或字段绑定配置不合法。
//	}
//
//	// binder.Fields 中会包含 ID 对应的 FieldBinding：
//	// FieldBinding{Index: []int{1}, Path: "id"}
//	request := new(GetOneTestctrl2Req)
//	if err := binder.Bind(ginContext, request); err != nil {
//		// 可以将 BindingError 转换为 HTTP 400 响应。
//	}
//
// 实际使用 ghttp.Server.Bind 注册 Controller 时，框架会自动调用
// NewBinder 和 Bind，业务代码通常不需要手动操作 Binder。
package router

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// BindingError describes a request decoding failure without coupling this
// package to the root server error type.
type BindingError struct {
	Code       int
	HTTPStatus int
	Message    string
	Cause      error
}

func (err *BindingError) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause == nil {
		return err.Message
	}
	return fmt.Sprintf("%s: %v", err.Message, err.Cause)
}
func (err *BindingError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// FieldBinding 是单个请求字段在启动阶段预先生成的绑定元数据。
//
// 通常由 NewBinder 创建，不需要业务代码手动构造。Index 表示请求结构体
// 中的字段位置，其余字段表示参数来源。例如：
//
//	ID int64 `path:"id"`
//
// 会生成 Path 为 "id" 的 FieldBinding，Binder.Bind 会读取 Gin 路由中的
// `:id` 参数，并将其转换为 int64。
type FieldBinding struct {
	// Index 是 reflect.StructField 的索引路径，用于定位请求结构体字段。
	// 嵌套或嵌入结构体的字段可能包含多个索引。
	Index []int
	// Path 是路由参数名，例如 `/users/:id` 对应 "id"。
	Path string
	// Query 是 URL 查询参数名，例如 `?page=2` 对应 "page"。
	Query string
	// Header 是 HTTP 请求头名称。
	Header string
	// Cookie 是请求 Cookie 名称。
	Cookie string
	// Form 是 URL 编码表单或 multipart 表单的字段名。
	Form string
	// File 是 multipart 上传文件的字段名。
	File string
}

// Binder 使用路由注册阶段生成的元数据执行请求数据绑定。
//
// Binder 会根据 Content-Type 处理 JSON 请求体，并处理 URL 查询参数、路径参数、
// 请求头、Cookie、表单字段和 multipart 文件。它负责将字符串转换为目标 Go 类型，
// 但不会执行 binding 或 validate 规则；校验由绑定成功后的 Server 阶段完成。
//
// 基本使用方式如下：
//
//	type GetUserRequest struct {
//		ID   int64 `path:"id"`
//		Page int   `query:"page"`
//	}
//
//	binder, err := router.NewBinder(reflect.TypeOf((*GetUserRequest)(nil)))
//	if err != nil {
//		// 请求类型或字段绑定配置无效。
//	}
//
//	request := new(GetUserRequest)
//	if err := binder.Bind(ginContext, request); err != nil {
//		// 可以将 BindingError 转换为 400 响应。
//	}
//
// NewBinder 同时会填充 Fields，集成代码可以在注册路由前检查生成的
// FieldBinding。由 NewBinder 创建的 Binder 默认将 multipart 解析内存限制为 32 MiB。
type Binder struct {
	Fields             []FieldBinding
	MaxMultipartMemory int64
	hasQuery           bool
	hasBindingRules    bool
	hasValidateRules   bool
}

// NewBinder 为指向请求结构体的指针类型创建 Binder，例如
// reflect.TypeOf((*GetUserRequest)(nil))。它会递归收集嵌入结构体中的绑定标签，
// 并检查 path、form、file 等字段类型是否支持。返回的 Binder 可以复用于同类型请求。
func NewBinder(requestType reflect.Type) (*Binder, error) {
	if requestType == nil || requestType.Kind() != reflect.Ptr || requestType.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("request type must be a pointer to a struct")
	}
	binder := &Binder{MaxMultipartMemory: 32 << 20}
	if err := collectFieldBindings(requestType.Elem(), nil, &binder.Fields); err != nil {
		return nil, err
	}
	for _, field := range binder.Fields {
		if field.Query != "" {
			binder.hasQuery = true
			break
		}
	}
	binder.hasBindingRules = hasValidationTag(requestType.Elem(), "binding", make(map[reflect.Type]struct{}))
	binder.hasValidateRules = hasValidationTag(requestType.Elem(), "validate", make(map[reflect.Type]struct{}))
	return binder, nil
}

// HasBindingRules reports whether the request type declares binding rules.
// The result is computed once during route registration for the request hot path.
func (binder *Binder) HasBindingRules() bool {
	return binder != nil && binder.hasBindingRules
}

// HasValidateRules reports whether the request type declares validation rules.
// The result is computed once during route registration for the request hot path.
func (binder *Binder) HasValidateRules() bool {
	return binder != nil && binder.hasValidateRules
}

func hasValidationTag(fieldType reflect.Type, tag string, seen map[reflect.Type]struct{}) bool {
	fieldType = indirectType(fieldType)
	if fieldType == nil || fieldType.Kind() != reflect.Struct {
		return false
	}
	if _, exists := seen[fieldType]; exists {
		return false
	}
	seen[fieldType] = struct{}{}
	defer delete(seen, fieldType)
	for fieldIndex := 0; fieldIndex < fieldType.NumField(); fieldIndex++ {
		field := fieldType.Field(fieldIndex)
		if raw, exists := field.Tag.Lookup(tag); exists && strings.TrimSpace(raw) != "" {
			return true
		}
		if hasValidationTag(field.Type, tag, seen) {
			return true
		}
	}
	return false
}

func collectFieldBindings(structType reflect.Type, prefix []int, fields *[]FieldBinding) error {
	for fieldIndex := 0; fieldIndex < structType.NumField(); fieldIndex++ {
		field := structType.Field(fieldIndex)
		index := append(append([]int(nil), prefix...), fieldIndex)
		if field.Anonymous && isMetaField(field.Type) {
			continue
		}
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		if field.Anonymous && isEmbeddedStruct(field.Type) && !hasBindingTags(field) {
			if err := collectFieldBindings(indirectType(field.Type), index, fields); err != nil {
				return err
			}
			continue
		}
		binding := FieldBinding{Index: index}
		binding.Path, _ = fieldTagName(field, "path", "")
		binding.Query, _ = fieldTagName(field, "query", "")
		binding.Header, _ = fieldTagName(field, "header", "")
		binding.Cookie, _ = fieldTagName(field, "cookie", "")
		binding.Form, _ = fieldTagName(field, "form", "")
		binding.File, _ = fieldTagName(field, "file", "")
		if binding.File != "" && !isMultipartFileType(field.Type) {
			return fmt.Errorf("field %s uses file binding but has unsupported type %s", field.Name, field.Type)
		}
		usesStringBinding := binding.Path != "" || binding.Query != "" || binding.Header != "" || binding.Cookie != "" || binding.Form != ""
		if usesStringBinding && !isMultipartFileType(field.Type) && !supportsStringBinding(field.Type) {
			return fmt.Errorf("field %s has unsupported binding type %s", field.Name, field.Type)
		}
		if binding.Path != "" || binding.Query != "" || binding.Header != "" || binding.Cookie != "" || binding.Form != "" || binding.File != "" {
			*fields = append(*fields, binding)
		}
	}
	return nil
}

// Bind 使用 Binder 中的 FieldBinding 将当前 Gin 请求解析到 target。
//
// target 必须是非 nil 的请求结构体指针。Content-Type 为 application/json 或
// +json 时解析 JSON；Content-Type 为 application/x-www-form-urlencoded 或
// multipart/form-data 时解析表单。path、query、header、cookie 和 form 参数
// 按 FieldBinding 读取，格式错误时返回 *BindingError。
//
// Bind 只负责数据解析；required 等校验规则由 Bind 成功后的 Server 阶段单独执行。
func (binder *Binder) Bind(ginContext *gin.Context, target any) error {
	request := ginContext.Request
	targetValue := reflect.ValueOf(target)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "request target must be a non-nil pointer"}
	}
	if len(binder.Fields) == 0 && (request.Body == nil || request.Body == http.NoBody) && request.Header.Get("Content-Type") == "" {
		return nil
	}
	contentType := normalizeContentType(request.Header.Get("Content-Type"))
	if isJSONContentType(contentType) && request.Body != nil {
		decoder := json.NewDecoder(request.Body)
		if err := decoder.Decode(target); err != nil && err != io.EOF {
			return jsonBindingError(err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				err = errors.New("request body must contain exactly one JSON value")
			}
			return jsonBindingError(err)
		}
	}
	if isMultipartContentType(contentType) {
		if err := request.ParseMultipartForm(binder.MaxMultipartMemory); err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				return &BindingError{Code: 41300, HTTPStatus: http.StatusRequestEntityTooLarge, Message: "multipart request body is too large", Cause: err}
			}
			return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "invalid form request body", Cause: err}
		}
	} else if isFormContentType(contentType) {
		if err := request.ParseForm(); err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				return &BindingError{Code: 41300, HTTPStatus: http.StatusRequestEntityTooLarge, Message: "form request body is too large", Cause: err}
			}
			return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "invalid form request body", Cause: err}
		}
	}
	var query url.Values
	if binder.hasQuery {
		query = request.URL.Query()
	}
	for _, field := range binder.Fields {
		value := fieldValue(targetValue.Elem(), field.Index, false)
		fileName := field.File
		if fileName == "" && field.Form != "" && value.IsValid() && isMultipartFileType(value.Type()) {
			fileName = field.Form
		}
		if fileName != "" && request.MultipartForm != nil {
			if files := request.MultipartForm.File[fileName]; len(files) > 0 {
				value = fieldValue(targetValue.Elem(), field.Index, true)
				if !value.IsValid() || !value.CanSet() {
					continue
				}
				if err := assignFiles(value, files); err != nil {
					return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: fmt.Sprintf("invalid file for %s", fileName), Cause: err}
				}
				continue
			}
		}
		bindStrings := func(name string, values []string) (bool, error) {
			if len(values) == 0 {
				return false, nil
			}
			if !value.IsValid() {
				value = fieldValue(targetValue.Elem(), field.Index, true)
			}
			if !value.IsValid() || !value.CanSet() {
				return false, nil
			}
			return assignBinding(value, name, values)
		}
		if field.Path != "" {
			bound, err := bindStrings(field.Path, pathValues(ginContext, field.Path))
			if err != nil {
				return err
			}
			if bound {
				continue
			}
		}
		if field.Query != "" {
			bound, err := bindStrings(field.Query, queryValues(query, field.Query))
			if err != nil {
				return err
			}
			if bound {
				continue
			}
		}
		if field.Header != "" {
			bound, err := bindStrings(field.Header, headerValues(request.Header, field.Header))
			if err != nil {
				return err
			}
			if bound {
				continue
			}
		}
		if field.Cookie != "" {
			bound, err := bindStrings(field.Cookie, cookieValues(request, field.Cookie))
			if err != nil {
				return err
			}
			if bound {
				continue
			}
		}
		if field.Form != "" {
			if _, err := bindStrings(field.Form, formValues(request, field.Form)); err != nil {
				return err
			}
		}
	}
	return nil
}

func assignBinding(destination reflect.Value, name string, values []string) (bool, error) {
	if name == "" || len(values) == 0 {
		return false, nil
	}
	if err := assignStrings(destination, values); err != nil {
		return true, &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: fmt.Sprintf("invalid value for %s", name), Cause: err}
	}
	return true, nil
}

func jsonBindingError(err error) *BindingError {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return &BindingError{Code: 41300, HTTPStatus: http.StatusRequestEntityTooLarge, Message: "request body is too large", Cause: err}
	}
	return &BindingError{Code: 40001, HTTPStatus: http.StatusBadRequest, Message: "invalid JSON request body", Cause: err}
}

func fieldValue(value reflect.Value, index []int, allocate bool) reflect.Value {
	for _, part := range index {
		for value.Kind() == reflect.Ptr {
			if value.IsNil() && allocate && value.CanSet() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			if value.IsNil() {
				return reflect.Value{}
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		value = value.Field(part)
	}
	return value
}

func assignStrings(destination reflect.Value, values []string) error {
	if destination.Kind() == reflect.Ptr {
		if destination.IsNil() {
			destination.Set(reflect.New(destination.Type().Elem()))
		}
		return assignStrings(destination.Elem(), values)
	}
	if destination.Kind() == reflect.Slice {
		if destination.Type().Elem().Kind() == reflect.Uint8 {
			destination.SetBytes([]byte(values[0]))
			return nil
		}
		result := reflect.MakeSlice(destination.Type(), len(values), len(values))
		for index := range values {
			if err := assignStrings(result.Index(index), []string{values[index]}); err != nil {
				return err
			}
		}
		destination.Set(result)
		return nil
	}
	if destination.Kind() == reflect.Array {
		if len(values) != destination.Len() {
			return fmt.Errorf("expected %d values, received %d", destination.Len(), len(values))
		}
		for index := range values {
			if err := assignStrings(destination.Index(index), []string{values[index]}); err != nil {
				return err
			}
		}
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	value := values[0]
	if destination.CanAddr() && destination.Addr().Type().Implements(textUnmarshalerType) {
		return destination.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value))
	}
	if destination.Type() == durationType {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		destination.SetInt(int64(parsed))
		return nil
	}
	switch destination.Kind() {
	case reflect.String:
		destination.SetString(value)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		destination.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(value, 10, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsed, err := strconv.ParseUint(value, 10, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value, destination.Type().Bits())
		if err != nil {
			return err
		}
		destination.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported binding type %s", destination.Type())
	}
	return nil
}

var (
	multipartFileHeaderType = reflect.TypeOf((*multipart.FileHeader)(nil))
	textUnmarshalerType     = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	durationType            = reflect.TypeOf(time.Duration(0))
)

func supportsStringBinding(valueType reflect.Type) bool {
	if valueType.Kind() == reflect.Ptr {
		return supportsStringBinding(valueType.Elem())
	}
	if reflect.PointerTo(valueType).Implements(textUnmarshalerType) || valueType == durationType {
		return true
	}
	switch valueType.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice, reflect.Array:
		return valueType.Elem().Kind() == reflect.Uint8 || supportsStringBinding(valueType.Elem())
	default:
		return false
	}
}

func isMultipartFileType(valueType reflect.Type) bool {
	return valueType == multipartFileHeaderType || (valueType.Kind() == reflect.Slice && valueType.Elem() == multipartFileHeaderType)
}
func assignFiles(destination reflect.Value, files []*multipart.FileHeader) error {
	if destination.Type() == multipartFileHeaderType {
		destination.Set(reflect.ValueOf(files[0]))
		return nil
	}
	if destination.Kind() == reflect.Slice && destination.Type().Elem() == multipartFileHeaderType {
		result := reflect.MakeSlice(destination.Type(), len(files), len(files))
		for index, file := range files {
			result.Index(index).Set(reflect.ValueOf(file))
		}
		destination.Set(result)
		return nil
	}
	return fmt.Errorf("unsupported multipart file type %s", destination.Type())
}
func pathValues(context *gin.Context, name string) []string {
	if name == "" {
		return nil
	}
	if value := context.Param(name); value != "" {
		return []string{value}
	}
	return nil
}
func queryValues(values url.Values, name string) []string {
	if name == "" {
		return nil
	}
	return values[name]
}
func headerValues(values http.Header, name string) []string {
	if name == "" {
		return nil
	}
	return values.Values(name)
}
func cookieValues(request *http.Request, name string) []string {
	if name == "" {
		return nil
	}
	cookie, err := request.Cookie(name)
	if err != nil {
		return nil
	}
	return []string{cookie.Value}
}
func formValues(request *http.Request, name string) []string {
	if name == "" {
		return nil
	}
	if len(request.PostForm) == 0 {
		_ = request.ParseForm()
	}
	if values := request.PostForm[name]; len(values) > 0 {
		return values
	}
	return request.Form[name]
}
func isJSONContentType(contentType string) bool {
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}
func isFormContentType(contentType string) bool {
	return contentType == "application/x-www-form-urlencoded"
}
func isMultipartContentType(contentType string) bool {
	return contentType == "multipart/form-data"
}
func normalizeContentType(contentType string) string {
	if separator := strings.IndexByte(contentType, ';'); separator >= 0 {
		contentType = contentType[:separator]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}
func fieldTagName(field reflect.StructField, tag, fallback string) (string, bool) {
	raw, ok := field.Tag.Lookup(tag)
	if !ok {
		return fallback, false
	}
	name := strings.TrimSpace(strings.Split(raw, ",")[0])
	if name == "-" {
		return "", true
	}
	if name == "" {
		return fallback, true
	}
	return name, true
}
func hasBindingTags(field reflect.StructField) bool {
	for _, tag := range []string{"path", "query", "header", "cookie", "form", "file", "json"} {
		if _, ok := field.Tag.Lookup(tag); ok {
			return true
		}
	}
	return false
}
func isEmbeddedStruct(fieldType reflect.Type) bool {
	return indirectType(fieldType).Kind() == reflect.Struct
}
func indirectType(fieldType reflect.Type) reflect.Type {
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}
	return fieldType
}
