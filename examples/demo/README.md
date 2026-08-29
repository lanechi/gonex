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
internal/database/           PostgreSQL 初始化与关闭
internal/logic/              业务实现和注册聚合
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
