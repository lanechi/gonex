---
name: gonex-use-data
description: 设计 gonex 应用的数据访问与启动基础设施；适用于数据库、Redis 等 bootstrap 组件、连接生命周期和事务边界。
---

# 使用 gonex 数据基础设施

## 启动与关闭

启动入口位于 `internal/bootstrap/`。数据库、Redis 等需要在 Server 前启动的组件，应按依赖顺序初始化，失败
立即返回；Server 停止时注册对应的关闭函数：

```go
if err := config.Init(); err != nil {
	return err
}
cfg := g.Cfg()
if err := db.InitializePostgres(cfg); err != nil {
	return err
}

server := ghttp.NewServer()
server.OnStop(func(context.Context) error {
	return db.ClosePostgres()
})
```

每种数据库使用自己的文件和命名，例如 `bootstrap/db/postgres.go` 的 `Postgres()`、
`InitializePostgres()`、`ClosePostgres()`。未启用的驱动文件保持注释，启用前补齐 module driver 依赖。

## 分层与事务

```text
Controller → Service → Logic → DAO / Repository → bootstrap client
```

- Controller 不持有数据库连接，也不直接执行 GORM 查询。
- Logic 编排业务事务、幂等和跨 DAO 写操作；不要让事务生命周期逃逸到 HTTP 层。
- DAO/Repository 操作必须接收并传递 `context.Context`。
- 连接初始化、关闭和失败回滚由 bootstrap/组合根负责；不要在请求处理中懒初始化全局连接。
- 外部客户端通过构造函数注入 Logic 或 Repository，避免业务包依赖具体 bootstrap 包。

## 配置与安全

数据库配置使用 `DATABASE_*` 环境变量或 `.env`；`gx dao` 和 demo PostgreSQL bootstrap 都不从
`config.yaml` 读取数据库连接。生产环境使用部署平台 Secret 注入环境变量，不部署含明文凭据的 `.env`。
连接失败、迁移失败和关闭失败都应保留错误链并返回给启动/关闭流程。
