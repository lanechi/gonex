---
name: gonex-use-data
description: 初始化 gonex 数据基础设施和全局客户端；适用于数据库、Redis 等 bootstrap、连接生命周期和关闭，不用于在 Controller 等业务层编写查询。
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
dao.SetDefault(db.Postgres())

server := ghttp.NewServer()
server.OnStop(func(context.Context) error {
	return db.ClosePostgres()
})
```

每种数据库使用自己的文件和命名，例如 `bootstrap/db/postgres.go` 的 `Postgres()`、
`InitializePostgres()`、`ClosePostgres()`。未启用的驱动文件保持注释，启用前补齐 module driver 依赖。

## 分层与事务

```text
Controller → Service → Logic → generated DAO → bootstrap database
```

- Controller 不持有数据库连接，也不直接执行 GORM 查询。
- 除本文件描述的 bootstrap 建连/关闭和 gx 生成 DAO 外，所有业务数据库操作只能位于 Logic。
- Logic 优先使用启动时初始化的 `dao.Q`；生成 DAO 无法表达时使用现有全局 DB accessor。两种路径都
  必须先调用 `WithContext(ctx)`，事务不得逃逸到 Logic 方法之外。
- 连接初始化、关闭和失败回滚由 bootstrap/组合根负责；不要在请求处理中懒初始化全局连接。
- API、Controller、Service、model、Middleware、Scheduler handler 不直接查询数据库，只调用 Service。
- 任何业务包都不得调用 `gorm.Open`、`sql.Open`，不得创建 DAO/Repository/DB wrapper 或缓存连接；
  配置统一读取 `g.Cfg()`，不保存配置或数据库状态。
- 项目已有的 Redis、对象存储等基础设施也复用其唯一全局入口；初始化和关闭只在 bootstrap/组合根，
  Logic 只取得已有 client，不在构造函数中创建第二个 client。数据库 fallback 可以通过
  `internal/bootstrap/db` 的全局 accessor 获取，但不能调用初始化或关闭函数。
- 跨层或跨 Logic 复用的导出结构体统一放 `internal/model`；生成 Entity 只用于持久化映射。

## 配置与安全

数据库配置使用 `DATABASE_*` 环境变量或 `.env`；`gx dao` 和 demo PostgreSQL bootstrap 都不从
`config.yaml` 读取数据库连接。生产环境使用部署平台 Secret 注入环境变量，不部署含明文凭据的 `.env`。
连接失败、迁移失败和关闭失败都应保留错误链并返回给启动/关闭流程。
