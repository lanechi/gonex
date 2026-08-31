---
name: gonex-use-logging
description: 在 gonex Controller、Service、后台任务和基础设施中使用结构化日志，并正确传递请求上下文。
---

# 使用 gonex 日志

## Controller 与 Service

Controller 和 Service 都应使用方法参数中的 `context.Context`：

```go
logger := logging.FromContext(ctx)
if logger != nil {
	logger.Info(ctx, "creating user", logging.Int64("user_id", id))
}
```

Controller 调用 Service 时继续传递原始 `ctx`，不要替换为 `context.Background()`，否则会丢失请求取消、
request ID 和请求级 Logger。

Service 不应依赖 `ghttp.Context`。如果 Service 可能脱离 HTTP 请求运行，允许在没有请求 Logger 时回退
到注入的 Logger 或 `logging.Default()`。

## 注入与字段

Server 日志可通过 `server.Logger()` 获取。复杂 Service、数据库或外部适配器优先在构造函数中注入
`logging.Logger`，不要直接依赖具体 Zap 类型：

```go
database := NewDatabase(server.Logger())
```

使用 `logging.String`、`logging.Int`、`logging.Bool`、`logging.Error` 和 `logging.Any` 添加结构化字段；
不要记录密码、Token、Cookie、完整请求体或支付数据。

全局 `g.SetLogger(logger)` 必须在创建第一个 Server 前调用；单个 Server 使用 `ghttp.WithLogger(logger)`。
