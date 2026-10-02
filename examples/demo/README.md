# gonex-demo

gonex-demo 是 `gx init` 使用的规范项目模板，提供可直接扩展的 gonex、Cobra、GORM 和 PostgreSQL
分层。请求链路为 API → Controller → gx 生成的 Service → Logic；数据库生命周期由应用管理。

## 开始使用

```bash
cp .env.example .env
go mod tidy
go run . serve
```

默认入口：

```text
GET http://localhost:8000/hello
GET http://localhost:8000/hello?name=Ada
GET http://localhost:8000/openapi.json
GET http://localhost:8000/docs/
```

启动服务需要可用的 PostgreSQL。可以设置 `DATABASE_DSN` 或 `DATABASE_URL`，也可以使用
`DATABASE_HOST`、`DATABASE_PORT`、`DATABASE_USER`、`DATABASE_PASSWORD`、`DATABASE_NAME`、
`DATABASE_SSLMODE` 和 `DATABASE_TIMEZONE` 组合连接信息。复制 `.env.example` 后填写真实值，不提交
`.env`。

## 目录

```text
api/                         HTTP Req/Res、g.Meta 和参数校验
config/                      Web Server 配置
internal/cmd/                Cobra 命令与应用组合根
internal/controller/         HTTP 边界和 gx Controller 契约（包文件、构造函数、动作实现）
internal/bootstrap/db/       启动基础设施；postgres.go 启用，mysql.go/sqlite.go 为注释模板
internal/logic/<name>/       业务实现；每个目录只放一个主要 Logic receiver
internal/model/              Controller、Service、Logic 复用的公共业务结构体
internal/model/entity/       gx dao 生成的数据库 Entity，不作为公共业务模型
internal/service/            gx 生成的 Service 接口
resource/public/             静态资源
resource/template/           HTML 模板
.agents/skills/              gonex 应用开发 skills（唯一 skills 目录）
.codex/                      项目 agents 与协作配置
```

## 开发资源

本地有 `gx` 时，修改 API 或 Logic 后先预览再同步：

```bash
gx ctrl --dry-run
gx service --dry-run
gx ctrl
gx service
```

带 `Code generated ... DO NOT EDIT.` 的 Controller 包文件、构造函数、Service 和聚合文件由 gx 维护；模块级 API 接口位于
`api/<module>/<module>.go`。API 请求/响应、
Controller 动作实现和 Logic 由开发者维护。新增完整资源时可调用 `$gonex-create-resource`；参数设计、
Controller、Service 和审查也有对应项目 skill。

`AGENTS.md` 已按任务类型强制路由这些 skills，每个 skill 的 `agents/openai.yaml` 也显式允许隐式调用。
`gx init` 会复制并校验完整 skill bundle，包括 `SKILL.md`、必需 references 和调用元数据，因此新项目会
继承相同规范。实现 Logic 或数据库业务时必须组合使用 `$gonex-implement-service` 与
`$gonex-use-dao`：除 bootstrap 初始化/关闭和 `gx dao` 生成外，所有查询、写入、事务和 Raw SQL 只能
位于 Logic，并优先使用已有 `dao.Q`，DAO 无法表达时使用项目现有全局 DB。

配置、模板和日志也有专用 skill：`$gonex-use-config` 说明 `config.yaml`、`.env`、系统环境变量
和启动初始化顺序；`$gonex-use-template` 说明 HTML 模板根目录、模板函数和页面渲染；
`$gonex-use-logging` 说明 Controller、Logic、后台任务和基础设施的结构化日志；
`$gonex-use-dao` 说明 `gx dao` 生成和使用 DAO/Entity；`$gonex-use-data` 说明数据库、Redis 等
启动基础设施和全局连接生命周期。配置统一通过 `g.Cfg()` 获取；禁止在业务代码中自行创建 DAO、
Repository、DB wrapper 或数据库连接。

## PostgreSQL 数组

`gx dao` 对 PostgreSQL 内置数组生成普通 Go slice，而不是 `pgtype.Array[T]` 或 `pq.*Array`：

```go
SourceURLs []string `gorm:"column:source_urls;type:text[];serializer:pgarray;pgarray:_text"`
ImageURLs  []string `gorm:"column:image_urls;type:text[];serializer:pgarray;pgarray:_text"`
VideoURLs  []string `gorm:"column:video_urls;type:text[];serializer:pgarray;pgarray:_text"`
```

只要 Entity package 中存在数组字段，gx 会同时生成 `pgarray_serializer.gen.go` 并自动注册
`pgarray` serializer。业务代码直接读写 `[]string`、`[]int64`、`[]bool` 等；serializer
只负责阻止 GORM 把 slice 展开成多个 SQL 参数，真正的 PostgreSQL array 编解码仍由 pgx ArrayCodec
完成。

`nil` slice 表示 SQL NULL，非 nil 的空 slice 表示 PostgreSQL 空数组。Create、查询、Save 和
Entity/struct Updates 可直接使用这些字段。不要使用
`Updates(map[string]any{"source_urls": []string{...}})` 更新数组：GORM 的 map update 不经过字段
serializer；GORM Gen 的 `UpdateColumn(..., []T)` / `UpdateSimple` 裸值路径也不会自动套 serializer。
需要更新数组时使用生成 DAO 的 `Updates(entity)`、GORM Entity/struct，或显式 SQL expression。

## 定时任务

需要应用内定时工作时，通过 `server.Scheduler().Add(scheduler.Job{...})` 在启动组合根注册。任务使用
`Cron`、`Every` 或 `Once`，必须命名并尊重传入的 `context.Context`；不要自行启动未跟踪的 goroutine。
Server 会在监听前调用 `Start(ctx)`，关闭时先 `Stop()` 取消运行中任务，再在 HTTP drain 后 `Wait(ctx)`。
`config.yaml` 可通过 `server.scheduler.enabled` 和 `server.scheduler.timezone` 配置本地调度器，但不声明
业务 Handler。持久化任务、分布式锁、重试和
业务队列仍属于应用基础设施，不由模板或 gx 生成。

## 验证

```bash
go test ./...
go vet ./...
git diff --check
```

完整代码规则见 [`AGENTS.md`](AGENTS.md)。
