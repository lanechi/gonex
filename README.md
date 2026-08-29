# gonex

gonex 是基于 Gin 的轻量 Go Web 框架。它用声明式 API、Controller、RouterGroup、请求绑定和统一
响应组织 Web 应用，同时让数据库、缓存、消息队列等业务基础设施保持可选。框架代码、`gx` 生成器、
规范模板、可运行 examples、文档和 AI skills 共同维护同一套公开契约。

## 文档入口

- [架构设计](docs/architecture.md)：设计目标、模块边界、注册与请求链路、状态所有权和演进原则。
- [开发规则](AGENTS.md)：代码规范、同步矩阵、生成代码边界和验证命令。
- [AI skills](examples/demo/.agents/skills/)：按当前框架规范创建 API、Controller、Logic、Service 和审查项目。
- [gx 代码生成器](gx/README.md)：`gx init/ctrl/service/dao` 的完整说明。
- [规范项目模板](examples/demo/README.md)：`gx init` 下载的 PostgreSQL 项目骨架。
- [可运行示例](examples/README.md)：`basic`、`quick-demo` 和 `template-demo`。
- [性能基准](benchmarks/gx/README.md)：gonex 与直接 Gin 的独立 benchmark。

## 当前能力

- 基于 Gin 的 HTTP Server、命名 Server 和独立 Server；
- 通过 `g.Meta` 声明路由与 OpenAPI 元数据；
- Controller 扫描、批量原子注册和框架自有路由表；
- `path`、`query`、`header`、`cookie`、JSON、`form`、`file` 请求绑定；
- `binding` 与 `validate` 两阶段校验；
- 应用级、分组级、Bind 级和路由级 Middleware；
- 可替换的统一响应编码器与错误处理器；
- Request ID、统一日志、Recovery、请求限制、Host、CORS 和 CSRF；
- 配置文件、`.env`、系统环境变量和运行时覆盖；
- Memory、签名 Cookie Session；Redis client 由业务持有，`contrib/redislog` 可接入 Redis 诊断日志；
- HTML 模板、热加载、静态目录、静态文件和 `fs.FS`；
- OpenAPI JSON、Swagger UI、TLS、生命周期 Hook、后台任务和优雅退出；
- `contrib/gormlog` GORM 日志适配器；
- 独立 `gx` module 提供项目、Controller、Service、DAO 和 Entity 生成。

## 要求与安装

项目当前使用 Go `1.26.0`。

安装框架：

```bash
go get github.com/lanechi/gonex
```

安装代码生成命令：

```bash
go install github.com/lanechi/gonex/gx@latest
gx --help
```

## 快速开始

### 使用生成器创建项目

```bash
gx init ./hello-gonex --module example.com/hello-gonex
cd hello-gonex
go mod tidy
go run . serve
```

`gx init` 下载与当前 `gx` 版本对应的 GitHub `examples/demo/` 模板，在 staging 中校验后替换 module 和项目名。
生成项目包含 API、Controller、Logic、Service、PostgreSQL 初始化、配置、README、AGENTS、Codex agents
和应用开发 skills，默认 Server 监听 `:8000`。

### 最小 HTTP Server

```go
package main

import (
	"context"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type HelloReq struct {
	g.Meta `path:"/hello" method:"get" tags:"Hello" summary:"Say hello"`
	Name   string `query:"name"`
}

type HelloRes struct {
	Message string `json:"message"`
}

type HelloController struct{}

func (*HelloController) Hello(_ context.Context, req *HelloReq) (*HelloRes, error) {
	name := req.Name
	if name == "" {
		name = "gonex"
	}
	return &HelloRes{Message: "Hello, " + name + "!"}, nil
}

func main() {
	server := ghttp.NewServer(ghttp.WithMode(ghttp.DebugMode))
	if err := server.Err(); err != nil {
		panic(err)
	}
	server.Group("/", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&HelloController{}); err != nil {
			panic(err)
		}
	})
	if err := server.Run(); err != nil {
		panic(err)
	}
}
```

启动后访问：

```text
GET http://localhost:8000/hello
GET http://localhost:8000/hello?name=Lane
GET http://localhost:8000/openapi.json
GET http://localhost:8000/docs/
```

默认成功响应：

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "message": "Hello, Lane!"
  }
}
```

## 路由与 Controller 契约

`Server.Engine()` 与 `Server.HTTPServer()` 是受控的底层集成出口：前者用于安装必须直接使用 Gin 的
第三方能力，后者用于调整标准库 HTTP Server 的非路由设置。应用不得通过它们直接注册业务路由、
替换 Handler 或改变框架已安装 Middleware 的顺序；这些操作会绕过框架 Registry、OpenAPI 与注册原子性。

Controller 的公开方法使用以下形式：

```go
func (*UserController) Create(
	ctx context.Context,
	req *CreateUserReq,
) (*CreateUserRes, error)
```

只有第二参数为请求结构体指针且该请求嵌入有效 `g.Meta` 的导出方法才会被识别为路由；Controller
中的普通导出辅助方法会被忽略。带有 `g.Meta` 但签名错误的方法仍会在注册阶段返回错误。

请求类型必须是结构体指针；方法、路径和文档元数据定义在嵌入的 `g.Meta` 上：

响应类型不要求必须是结构体指针。只要最终能由 JSON 编码器编码，就可以使用普通值或指针，包含命名
slice、map、标量和 struct，例如：

```go
type AppUserReviewsRes []model.AppEvaluationReview

func (*ReviewController) List(context.Context, *ListReviewsReq) (AppUserReviewsRes, error) {
	return AppUserReviewsRes{}, nil
}
```

`gx ctrl` 默认仍生成 `*Res` 签名以保持生成项目的兼容性；手写 Controller 可以按接口契约选择值或指针。

```go
type CreateUserReq struct {
	g.Meta `path:"/users/:id" method:"post" tags:"User" summary:"Create user"`

	ID      int64  `path:"id" binding:"required"`
	TraceID string `header:"X-Trace-ID"`
	Name    string `json:"name" binding:"required"`
	Age     int    `json:"age" validate:"gte=0"`
}
```

Server 可以直接 Bind，也可以通过分组批量 Bind：

```go
server.Group("/api", func(group *ghttp.RouterGroup) {
	group.Middleware(authMiddleware)
	if err := group.Bind(&UserController{}, &OrderController{}); err != nil {
		panic(err)
	}
})
```

框架会在修改真实 Gin 路由树前扫描并校验整个批次。路径字段缺失、重复路由、参数路由冲突或
`nil` Middleware 都会在注册阶段返回错误，不会留下部分注册。

路径参数校验针对最终路由执行，因此 RouterGroup 前缀也可以声明路径参数：例如
`Group("/tenants/:tenant")` 下的请求结构体可使用 `path:"tenant"` 绑定该参数；Controller 自身的
`g.Meta.path` 不要重复写入分组前缀。

## 请求绑定与校验

字段来源：

| 标签 | 来源 |
| --- | --- |
| `path:"id"` | 路由参数 |
| `query:"page"` | URL 查询参数 |
| `header:"X-Token"` | HTTP Header |
| `cookie:"session"` | Cookie |
| `form:"name"` | URL-encoded 或 multipart 表单 |
| `file:"avatar"` | multipart 上传文件 |
| `json:"name"` | JSON 请求体 |

`FieldBinding` 只保存 `path`、`query`、`header`、`cookie`、`form` 和 `file` 的注册期元数据。
JSON 请求体由 Content-Type 和标准 `json` 标签解码，不会作为 `FieldBinding` 条目保存：

```bash
curl -X POST http://localhost:8000/users/1 \
  -H 'Content-Type: application/json' \
  -d '{"name":"Lane","age":18}'
```

`binding` 用于绑定阶段的必填规则，`validate` 用于长度、范围、格式等字段规则；错误默认转换为统一的
HTTP 400 响应。`WithValidator` 注入配置完成的 `validate` Validator；需要自定义 `binding` 规则时，
用 `WithBindingValidator` 另行注入已设置 `binding` tag name 的独立实例。Server 将两个实例视为只读，
不会在请求期间切换标签或清理缓存；错误字段保留嵌套 Go 字段路径。

查询、Header、Cookie 和表单参数可以使用 `default`（或短别名 `d`）设置可选默认值。参数缺失时，Binder 会先应用默认值，
再执行 `binding` 与 `validate`；显式传入的值不会被覆盖。例如 `Page int \`query:"page" default:"1"\``。
路径参数不能设置默认值，JSON 请求体中的 `default`/`d` 目前仅用于 OpenAPI 描述。

## Middleware

Middleware 执行顺序为：

```text
系统级 → Server.Use → RouterGroup/Bind → Route.Use → Controller
```

应用级 Middleware 必须在第一次 Bind 前注册：

```go
if err := server.Use(requestMetrics); err != nil {
	panic(err)
}

if err := server.Route("GET", "/api/users/:id").Use(requireOwner); err != nil {
	panic(err)
}
```

`Route.Use` 使用最终路径，包括 Group 前缀，也必须在目标 Controller Bind 前调用。

## 错误与自定义响应

业务错误使用结构化错误：

```go
return nil, ghttp.NewError(10001, 404, "user not found")
```

默认错误响应使用 `code` 和 `message`。`Details` 仅在 debug 模式返回，release/test 模式不会暴露
内部实现信息。可以通过 `ghttp.WithResponseEncoder` 和 `ghttp.WithErrorHandler` 替换成功与失败出口。

## 配置

默认搜索以下配置文件，找到第一个后停止：

```text
config.yaml
config/config.yaml
manifest/config/config.yaml
```

项目根目录 `.env` 会自动加载。有效优先级为：

```text
config.Set > 系统环境变量 > .env > 配置文件 > 默认值
```

嵌套键使用大写下划线环境变量，例如 `server.address` 对应 `SERVER_ADDRESS`。`Unmarshal` 会从目标
结构发现嵌套键，因此只存在于系统环境变量或 `.env` 的值也能进入嵌套结构；运行期 `Set`、`Get` 和
`Unmarshal` 可以并发调用。

```yaml
server:
  address: ":8000"
  mode: release
  # 可选：默认启用；Cron 使用 IANA 时区。
  scheduler:
    enabled: true
    timezone: America/Los_Angeles
  openapi:
    enabled: true
    path: /openapi.json
  swagger:
    path: /docs

logger:
  level: info
  format: json
  output: stdout
```

Server Option 高于配置文件。配置或 Option 无效时，通过 `server.Err()`、`Bind` 或 `Run` 返回错误。

## OpenAPI、日志与安全默认值

OpenAPI JSON 与 Swagger UI 共用一个开关：

```go
server := ghttp.NewServer(
	ghttp.WithOpenAPI(ghttp.OpenAPIOptions{
		Enabled:      true,
		DocumentPath: "/api/openapi.json",
		SwaggerPath:  "/api/docs",
	}),
)
```

`WithOpenAPI(ghttp.OpenAPIOptions{Enabled: false})` 会同时关闭 JSON 与 UI。Swagger 模板可通过
`openapi.SetSwaggerTemplate` 全局设置，模板使用 `{SwaggerUIDocUrl}` 占位符。

框架日志统一通过 `logging.Logger`；Zap 只是默认实现。自定义 Logger 应在第一次创建 Server 前
通过 `g.SetLogger`、`logging.SetLogger` 或 `ghttp.WithLogger` 注入。

主要安全默认值：

- 默认不信任代理，调用 `SetTrustedProxies` 后才启用；
- CORS 开启时必须配置至少一个 `AllowOrigins`，不会默认放行 `*`；
- Session/CSRF 的 `SameSite=None` 必须同时启用 Secure；
- 默认限制 Body、multipart 内存和 Header 大小；
- release/test 响应不包含内部错误详情。

## Session、模板与静态资源

同一请求内多次调用 `ghttp.FromContext(ctx).Session()` 会返回同一个 Session，Middleware 与
Controller 可以观察到彼此在该请求中的修改。`Logout` 会使旧 Session handle 失效并驱逐请求缓存；
同一请求后续再次调用 `Session()` 会打开独立的新会话。存储后端可选实现
`session.Storage` 是 context-first 存储边界，所有持久化操作都会接收请求 Context。框架不提供
Redis 驱动的存储或 Redis client 构造；业务可按自身生命周期实现 `session.Storage` 并持有 client。
如需把 go-redis 诊断日志接入框架 Logger，使用不持有 client 的 `contrib/redislog` 适配器。

签名 Cookie Session 每次修改都会轮换 Token，并在当前进程内撤销旧 Token；Logout 撤销整个会话族，
防止此前签发的 Token 被重放。撤销表是进程内状态，多副本或跨重启的强制注销应由业务提供共享的
服务端存储。

`Server.AddTemplateFunc` 会拒绝非法名称、nil 和不符合 `html/template` 约定的签名并返回 error，
不会把非法函数写入模板管理器。静态目录、文件和 `fs.FS` 挂载会同时执行路径边界检查与扩展名
白名单，并先在临时 Gin 路由树中验证 GET/HEAD，冲突失败不会残留半条路由。

默认白名单包含 HTML、JS、CSS、常见图片、字体、Wasm 和 Web App Manifest，不包含文本、源码、
配置、source map 等文件。`StaticOptions.Extensions == nil` 使用默认值；显式空切片拒绝全部；自定义
扩展名大小写不敏感。SPA fallback 只能回退到白名单允许的 index，禁止的请求扩展名不会借 fallback
读取页面。所有本地目录挂载还会拒绝绝对路径、编码/反斜杠越界和符号链接逃逸。配置式挂载可用
`server.static.extensions` 设置同一白名单；`[]` 表示拒绝全部。

## 定时任务

每个 `Server` 都拥有独立的 `scheduler.Scheduler`。它在 `OnStart` 阶段调用 `Start(ctx)`；优雅关闭先
调用 `Stop()` 取消任务 Context，再在 HTTP 连接 drain 后调用 `Wait(ctx)`。应用不需要自行管理这三步。
可通过 `server.scheduler.enabled` 禁用执行；禁用时 `Scheduler()` 仍可注册和查看任务，但 Server 不会启动它。
`server.scheduler.timezone` 为未带 `TZ=`/`CRON_TZ=` 的 Cron 设置
IANA 时区；无效时区会通过 `server.Err()` 返回。

```go
if err := server.Scheduler().Add(scheduler.Job{
	Name:     "sync-users",
	Schedule: scheduler.Every{Duration: 10 * time.Minute},
	Timeout:  30 * time.Second,
	Handler: func(ctx context.Context) error {
		return users.Sync(ctx)
	},
}); err != nil {
	return err
}
```

`scheduler.Cron{Expr: "0 0 3 * * *"}` 支持带秒的六字段表达式，也支持标准五字段表达式；
`scheduler.Once{At: time.Now().Add(time.Minute)}` 只在未来时刻执行一次。任务名在单个 Server 内唯一。
默认 `SkipIfRunning` 防止同一任务重入；可显式选择 `AllowOverlap` 或 `QueueOne`。任务 panic 会被恢复并
记录到 Server Logger，`Timeout` 和 Server 关闭都会取消传给 `Handler` 的 Context。通用拦截器可通过
`server.Scheduler().Use(...)` 注册；`WithScheduler` 仅用于注入一个有意自定义的 Scheduler。注入后由该
Server 独占其 Start/Stop/Wait 生命周期；调用方不得将同一实例注入多个 Server。

## 多 Server 与生命周期

命名 Server：

```go
api := g.Server("api")
admin := g.Server("admin")
```

完全独立的 Server：

```go
server := ghttp.NewServer(
	ghttp.WithName("worker-api"),
	ghttp.WithAddress(":8002"),
)
```

路由、OpenAPI、Logger、Session、模板、scheduler 和生命周期状态按 Server 隔离。Gin mode 是进程级设置，
同一进程不能让不同 Gin Server 使用不同 mode。

资源初始化和释放使用生命周期：

```go
server.OnStop(func(ctx context.Context) error {
	return database.Close()
})

server.Go(func(ctx context.Context) {
	// 在 ctx 取消后退出后台任务。
})
```

`Run` 处理 `SIGINT/SIGTERM` 并优雅退出；需要外部控制时使用 `RunContext` 或 `Shutdown`。
`OnStart`/`OnStarted` Hook 只有全部成功后才提交对应阶段的状态。独立的 `lifecycle.Lifecycle`
阶段调用在失败后可以重试，并发调用会等待同一轮结果；HTTP `Server.Run*` 不支持并发调用，
运行中再次调用会返回 `ErrServerRunning`。Server 启动失败时会执行完整关闭清理，该实例不再复用；
应用应修正问题并创建新的 Server 实例重试。

## 包边界

| 包 | 用途 |
| --- | --- |
| `g` | 常用入口和 `g.Meta` |
| `ghttp` | 应用通常直接使用的 HTTP Server API |
| `router` | 路由与绑定契约，主要供框架和集成层使用 |
| `config`、`logging` | 可替换的配置与日志基础 |
| `middleware` | 可复用的底层安全与请求 Middleware |
| `session`、`cookie` | Session 存储和 Cookie 能力 |
| `template`、`static` | 模板与静态资源 |
| `openapi` | 文档生成与 Swagger HTML |
| `contrib/gormlog` | 可选 GORM 日志适配器 |

业务数据库不是 `ghttp.Server` 的内置依赖；应用自行初始化并注入。规范 `examples/demo/` 和 `gx init` 项目
只提供 PostgreSQL 初始化，应用可按自己的基础设施边界替换。

## 开发与验证

核心 module：

```bash
go test ./...
go test -race ./...
go vet ./...
```

`gx`、每个 example 和 benchmark 都有独立 `go.mod`，必须进入相应目录执行命令。完整矩阵见
[开发规则](AGENTS.md)。

项目采用强制同步策略：公共行为变化必须同时检查核心测试、`gx` 生成器、相关 example、README
和架构文档；不能只让核心代码与生成项目分叉。核心公共包的导出声明同时由
`test/testdata/public_api.golden` 冻结；有意调整 API 时必须审查差异后显式更新该基线。
