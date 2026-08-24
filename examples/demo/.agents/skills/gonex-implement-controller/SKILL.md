---
name: gonex-implement-controller
description: 实现或重构 gonex Controller 动作、HTTP 错误边界、请求上下文和路由绑定；适用于 API 已定义后连接 Service 的工作。
---

# 实现 gonex Controller

开始实现前阅读 [references/controller-patterns.md](references/controller-patterns.md)，并检查目标模块的
API 类型、生成契约和相邻 Controller。

## 实现契约

Controller 方法保持：

```go
func (*ControllerV1) Action(
    ctx context.Context,
    req *v1.ActionReq,
) (*v1.ActionRes, error)
```

- 接收已绑定、已校验的 `req`；不要重复解析 path/query/JSON。
- 映射为领域输入，调用 `service.<Module>()`，再映射为公开响应。
- 透传 `context.Context`，不要替换成 `context.Background()`。
- 预期业务错误转换为带稳定业务码和 HTTP 状态的 `ghttp.Error`；保留 `Cause` 供日志和错误链使用。
- 普通 JSON 成功响应只返回 `*Res`。只有文件、流、重定向、HTML 或特殊状态码才通过
  `ghttp.FromContext(ctx)` 直接写响应；写出后返回 `nil, nil`，避免第二份响应。

## 边界

Controller 不直接访问 GORM、DAO、数据库事务或具体 Logic 类型，不放置可复用业务规则。Logger、
Session、HTML 和底层 `net/http` 集成从 `ghttp.FromContext(ctx)` 获取；除非第三方集成确实要求，
不要依赖 `Gin()`。

修改实现前确认文件没有 `DO NOT EDIT` 标记。若契约缺少动作，先修改 API 并运行
`gx ctrl --dry-run`、`gx ctrl`；不要手改 Controller 的 `*_generated.go`。

## 验证

为成功、绑定/校验失败、Service 错误映射及授权边界增加 HTTP 或 Controller 测试。运行 `gofmt`、
目标 module 测试，并确认响应只提交一次、OpenAPI 路由仍与实现一致。
