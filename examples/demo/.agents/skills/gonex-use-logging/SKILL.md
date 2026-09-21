---
name: gonex-use-logging
description: 在 gonex Controller、Logic、后台任务和基础设施中使用结构化日志，并正确传递请求上下文。
---

# 使用 gonex 日志

## Controller 与 Logic

Controller 和 Logic 都应使用方法参数中的 `context.Context`：

```go
logger := logging.FromContext(ctx)
if logger != nil {
	logger.Info(ctx, "creating user", logging.Int64("user_id", id))
}
```

Controller 调用 Service 时继续传递原始 `ctx`，不要替换为 `context.Background()`，否则会丢失请求取消、
request ID 和请求级 Logger。

Service/Logic 不应依赖 `ghttp.Context`。如果任务可能脱离 HTTP 请求运行，在没有请求 Logger 时统一
回退到 `logging.Default()`。

## 全局入口与字段

应用只维护一套默认 Logger。启动时需要替换默认实现，应在创建 Server、数据库和后台任务前调用
`g.SetLogger(logger)`；业务 Logic 和基础设施复用 `logging.FromContext(ctx)` / `logging.Default()`，
不在每个构造函数中注入或缓存另一份 Logger，也不直接依赖 Zap 类型。

```go
g.SetLogger(logger)
server := ghttp.NewServer()
```

使用 `logging.String`、`logging.Int`、`logging.Bool`、`logging.Error` 和 `logging.Any` 添加结构化字段；
不要记录密码、Token、Cookie、完整请求体或支付数据。

仅确实需要隔离 Logger 的独立 Server 测试使用 `ghttp.WithLogger(logger)`；应用业务代码仍遵循全局入口。
