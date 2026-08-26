# gonex Controller 模式

## 1. 文件与契约

`gx ctrl` 通常在 `internal/controller/<module>` 产生两类文件：

- `<module>_<version>_generated.go`：Controller 类型、构造函数和接口契约，gx 拥有；
- `<module>_<version>_<action>.go`：动作实现，首次创建后由开发者维护。

文件头存在 `DO NOT EDIT` 时不得手改，即使文件名看起来像实现文件。先修改 API，再运行生成器。

方法名、请求和响应必须与生成契约一致。标准 JSON 动作使用：

```go
func (*ControllerV1) CreateUser(
	ctx context.Context,
	req *v1.CreateUserReq,
) (*v1.CreateUserRes, error) {
	input := &model.CreateUserInput{
		Name:  req.Name,
		Email: req.Email,
	}

	user, err := service.User().Create(ctx, input)
	if err != nil {
		return nil, mapUserError(err)
	}
	return &v1.CreateUserRes{ID: user.ID, Name: user.Name}, nil
}
```

模型名以项目实际定义为准。不要为了照抄示例创建重复 DTO。

## 2. 错误边界

Logic 返回领域错误；Controller 决定公开业务码、HTTP 状态和安全消息：

```go
func mapUserError(err error) error {
	if errors.Is(err, model.ErrUserNotFound) {
		frameworkErr := ghttp.NewError(40401, http.StatusNotFound, "user not found")
		frameworkErr.Cause = err
		return frameworkErr
	}
	frameworkErr := ghttp.NewError(50001, http.StatusInternalServerError, "internal server error")
	frameworkErr.Cause = err
	return frameworkErr
}
```

- 使用稳定、项目内唯一或有命名空间规则的业务码。
- 不把 SQL、文件路径、凭据或内部错误文本作为 release 响应消息。
- 用 `errors.Is` / `errors.As` 保留错误链；未知错误映射为安全的 500。
- 绑定和结构校验已由框架处理，不要把它们再次映射为业务错误。

如果项目已有统一错误映射器，复用它而非在每个 Controller 复制 switch。

## 3. 请求 Context

始终把方法收到的 `ctx` 传给 Service、DAO 间接调用和外部 I/O。请求级能力通过：

```go
requestContext := ghttp.FromContext(ctx)
```

可使用 `Logger()`、`Session()`、`RequestID()`、`HTML()`、`File()`、`Stream()`、`Redirect()`、
标准 Request/Response。仅第三方库确实需要 Gin 时调用 `Gin()`，避免业务代码绑定执行引擎。

同一请求多次调用 `Session()` 返回同一实例，因此 Middleware 写入的值可被 Controller 读取；调用
`Logout` 后旧 handle 不得继续写入，再次需要 Session 时从 Context 重新获取。不要把
Session 保存到请求之外；自定义远程存储应实现 context-aware storage，让取消和 tracing 进入 I/O。

## 4. 直接响应

普通 JSON 返回 `Res` 或 `*Res`，也可以返回命名 slice、map、标量等 JSON 可编码类型，由统一 ResponseEncoder
包装。以下场景可直接写：

```go
func (*ControllerV1) Download(
	ctx context.Context,
	req *v1.DownloadReq,
) (*v1.DownloadRes, error) {
	asset, err := service.Asset().ResolveDownload(ctx, req.ID)
	if err != nil {
		return nil, mapAssetError(err)
	}
	requestContext := ghttp.FromContext(ctx)
	requestContext.File(http.StatusOK, asset.Path)
	return nil, nil
}
```

直接写响应后必须返回 `nil` 错误；若签名含响应值则返回 `nil, nil`。否则统一编码器或错误处理器可能
尝试提交第二份响应。文件路径不能直接信任用户输入，必须由 Logic 解析成授权后的安全资源。

## 5. 路由与 Middleware

确认 Controller 实例被应用现有的 `Server.Bind` 或 `RouterGroup.Bind` 路径注册。Middleware 语义为：

```text
系统级 → Server.Use → RouterGroup / Bind → Route.Use → Controller
```

认证、租户和速率限制放在最窄且可复用的正确层级。不要在动作内部重做已经由 Middleware 保证的
机械检查；但资源级授权仍应在 Logic 中基于已认证身份执行。

## 6. 测试

优先使用 `httptest` 绑定真实 Server，验证：

- 路由、方法和分组前缀；
- path/query/header/cookie/JSON/form/file 绑定；
- binding/validate 失败不会调用 Logic；
- 领域错误到业务码和 HTTP 状态的映射；
- release 模式不泄漏 Cause/Details；
- 直接响应不会出现第二个 JSON 包络；
- Controller 注册后 OpenAPI 与请求模型一致。

纯 Controller 单元测试若调用了 `ghttp.FromContext`，需要构造真实请求上下文；此时 HTTP 集成测试
通常更简单、更接近实际契约。
