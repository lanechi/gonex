# gonex

gonex 是一个基于 Gin 的轻量 Go Web 框架，设计思路来源于 GoFrame v2，但只聚焦 Web Server：声明式路由、Controller、请求绑定、统一响应、Middleware、配置、日志、安全、Session、模板、静态资源、OpenAPI、生命周期和定时任务。

数据库、缓存、消息队列不是核心运行时依赖。业务项目自行持有这些基础设施的 client 与生命周期。`contrib/gormlog` 与 `contrib/redislog` 各自是独立 Go module，因此 GORM/Redis 不进入 core module 的依赖图；代码生成器 `gx` 也是独立 Go module。

> **当前处于 v1 之前。** 框架优先保证 API 简洁、语义正确、安全和并发行为可证明，不为了兼容错误或冗余 API 增加 alias、函数转发或兼容层。破坏式修改必须同步测试、示例和文档。

## 文档入口

- [架构设计](docs/architecture.md)
- [开发与协作规则](AGENTS.md)
- [gx 代码生成器](gx/README.md)
- [examples](examples/README.md)
- [规范项目模板](examples/demo/README.md)
- [性能基准](benchmarks/gx/README.md)
- [应用开发 skills](examples/demo/.agents/skills/)

## 当前能力

- Gin HTTP Server、命名 Server、独立 Server；
- `g.Meta` 声明路由和 OpenAPI 元数据；
- Controller 扫描、批次预校验、Registry 与 Gin 注册；
- `path`、`query`、`header`、`cookie`、JSON、`form`、`file` 请求绑定；
- `binding` 与 `validate` 两阶段校验；
- Server、Group、Bind、Route 四级 Middleware；
- 可替换响应编码器和错误处理器；
- Request ID、结构化日志、Recovery、Host、CORS、CSRF、请求大小限制；
- 配置文件、`.env`、环境变量、运行时配置覆盖；
- Memory Session、签名 Cookie Session、自定义 `session.Storage`；
- HTML 模板、热加载、静态目录、静态文件、`fs.FS`；
- OpenAPI JSON、Swagger UI、TLS；
- 生命周期 Hook、可取消后台任务、优雅退出；
- 本地 Scheduler、持久任务 Loader、Singleton 分布式锁接口和运行记录接口；
- `contrib/gormlog`、`contrib/redislog`；
- 独立 `gx` module：项目、Controller、Service、DAO、Entity 生成。

## 要求与安装

当前使用 Go `1.26.0`。

```bash
go get github.com/lanechi/gonex
```

安装 `gx`：

```bash
go install github.com/lanechi/gonex/gx@latest
gx --help
```

按需安装第三方日志适配：

```bash
go get github.com/lanechi/gonex/contrib/gormlog
go get github.com/lanechi/gonex/contrib/redislog
```

这两个 `contrib` 包是独立 module，不会把 GORM/Redis 加入只使用 gonex core 的应用依赖图。

核心 module 与 `gx` 使用独立版本标签：

```text
核心: vX.Y.Z
gx:   gx/vX.Y.Z
```

## 快速开始

### 用 gx 创建项目

```bash
gx init ./hello-gonex --module example.com/hello-gonex
cd hello-gonex
go mod tidy
go run . serve
```

### 最小 Server

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
    if err := server.Bind(&HelloController{}); err != nil {
        panic(err)
    }
    if err := server.Run(); err != nil {
        panic(err)
    }
}
```

默认文档地址：

```text
GET /openapi.json
GET /docs/
```

OpenAPI `info.title` 默认使用 Server name；`info.version` 默认是 `unversioned`。应用可通过 `server.openapi.title` 和 `server.openapi.version` 显式声明文档身份。未单独声明的应用错误状态由 OpenAPI `default` response 覆盖。

## Controller、Router 与 Binder

Controller 路由方法使用：

```go
func (*UserController) Create(
    ctx context.Context,
    req *CreateUserReq,
) (*CreateUserRes, error)
```

请求参数必须是结构体指针并嵌入准确的 `g.Meta`。`g.Meta` 是 `router.Meta` 的类型别名，路由扫描只接受这个精确类型，不接受其它包中同名的 `Meta`。

响应可以是 JSON 可编码的 struct、slice、map、标量及其指针；不是只允许 `*Res`。

```go
type ReviewList []Review

func (*ReviewController) List(context.Context, *ListReq) (ReviewList, error) {
    return ReviewList{}, nil
}
```

`router.Registry` 是框架自己的路由事实来源；Gin 是执行后端。Controller 批次在写入真实 Gin 路由树之前会先完成：

1. Controller/Meta 扫描；
2. Binder 编译；
3. 路径参数校验；
4. Handler 数量校验；
5. Registry 冲突校验；
6. 临时 Gin 路由树冲突校验。

失败不会把一个 Controller 批次部分注册到框架路由表。

Binder 的运行时编译计划是 `router` 私有状态。外部需要检查路由时使用 `Server.Routes()` / `router.RouteMetadata`，不要依赖或修改 Binder 内部字段。multipart 内存上限由 Server 在请求执行时传入 Binder，不存放在可变 Binder 对象中。

### 绑定来源

| 标签 | 来源 |
| --- | --- |
| `path:"id"` | Gin 路径参数 |
| `query:"page"` | URL Query |
| `header:"X-Token"` | Header |
| `cookie:"session"` | Cookie |
| `form:"name"` | form / multipart |
| `file:"avatar"` | multipart 文件 |
| `json:"name"` | JSON body |

`binding` 用于绑定阶段约束，`validate` 用于业务字段校验。`default` / `d` 可为 Query、Header、Cookie、Form 等缺省字段提供默认值；路径参数不能设置默认值。

## Middleware

执行顺序：

```text
系统级 → Server.Use → RouterGroup/Bind → Route.Use → Controller
```

```go
if err := server.Use(requestMetrics); err != nil {
    panic(err)
}

server.Group("/api", func(group *ghttp.RouterGroup) {
    group.Middleware(authMiddleware)
    if err := group.Bind(&UserController{}); err != nil {
        panic(err)
    }
})

if err := server.Route("GET", "/admin/users/:id").Use(requireAdmin); err != nil {
    panic(err)
}
```

会改变路由拓扑或 Handler 顺序的配置必须在对应路由 Bind 前完成。`nil` Middleware 会被拒绝。

## 配置

默认搜索：

```text
config.yaml
config/config.yaml
manifest/config/config.yaml
```

优先级：

```text
config.Set > 系统环境变量 > .env > 配置文件 > 默认值
```

`server.address` 对应环境变量 `SERVER_ADDRESS`。`ViperConfig` 内部访问使用锁，`Get`、`Set`、`Unmarshal` 可以并发调用。

通过 `WithConfig` 注入自定义 `Config` 时，生命周期和并发安全由该实现负责。

## 安全默认值与运行期设置

- Gin trusted proxies 默认清空；只有调用 `SetTrustedProxies` 才信任代理；
- CORS 启用时必须提供允许来源；credentials 不能配合 `*`；
- `SameSite=None` 的 Session/CSRF Cookie 必须为 Secure；
- Body、multipart memory、Header 都有默认上限；
- release/test 模式不向客户端返回内部错误详情；
- 静态资源有路径边界、symlink 和扩展名约束；静态挂载属于 Gin topology mutation，必须在 `Run*` 前完成，并与启动通过 `registrationMu` 序列化；
- 运行期 Host/CORS/CSRF 设置通过快照与锁切换，不把调用方 slice 直接暴露给请求热路径。

```go
server.SetAllowedHosts("api.example.com")

if err := server.EnableCORS(ghttp.CORSOptions{
    Enabled:      true,
    AllowOrigins: []string{"https://app.example.com"},
}); err != nil {
    panic(err)
}
```

## Session

Session 采用 **JSON-safe、detached snapshot** 契约。

可写入的值必须能由 `encoding/json` 表达；例如 string、bool、number、slice、map、struct。func、chan、循环引用、NaN/Inf 等会返回错误。

```go
current, err := ghttp.FromContext(ctx).Session()
if err != nil {
    return err
}

if err := current.Set("profile", map[string]any{
    "name": "Lane",
    "roles": []string{"user"},
}); err != nil {
    return err
}
```

框架在 Session 边界规范化并复制值：

- `Set` 不保留调用方 map/slice 的引用；
- `Get` 返回独立快照，修改返回值不会修改 Session 内部状态；
- MemoryStorage、CookieStorage 和 `managedSession` 使用一致的 canonical JSON 值语义；
- JSON 数字使用 `json.Number` 保存，避免大整数被 `float64` 静默丢精度；
- 自定义 `session.Storage` 返回的数据在进入请求 Session 前也会被规范化。

`session.Storage` 是 context-first 接口：

```go
type Storage interface {
    Get(context.Context, string) (map[string]any, error)
    Set(context.Context, string, map[string]any, time.Duration) error
    Delete(context.Context, string) error
}
```

Redis client 等外部资源由业务持有，不放入 gonex core。

## Scheduler

### 本地任务

Server 默认拥有一个 Scheduler，并随 Server 生命周期 Start/Stop/Wait。

```go
err := server.Scheduler().Add(scheduler.Job{
    Name:     "cache.cleanup",
    Schedule: scheduler.Every{Duration: 10 * time.Minute},
    Handler: func(ctx context.Context) error {
        return cleanup(ctx)
    },
    Timeout:       30 * time.Second,
    OverlapPolicy: scheduler.SkipIfRunning,
})
if err != nil {
    panic(err)
}
```

支持：

- `Cron{Expr: ...}`：5 或 6 字段；
- `Every{Duration: ...}`；
- `Once{At: ...}`；
- `SkipIfRunning`（默认）、`AllowOverlap`、`QueueOne`；
- job/global Middleware；
- `RunImmediately`；
- per-job Timeout；
- Server Context 取消和优雅退出。

`Once + RunImmediately` 是无意义的双触发组合，会被拒绝。

### MutableScheduler

持久任务同步需要比普通 `Scheduler` 更强的原子语义：

```go
type MutableScheduler interface {
    Scheduler
    Validate(Job) error
    Replace(Job) error
}
```

`Replace` 必须在替换失败时保留旧任务，并跨版本保留同一个 overlap gate，避免任务更新时绕过 `SkipIfRunning` / `QueueOne`。

### 持久任务 Loader

持久任务核心不依赖 GORM、Redis 或具体数据库：

```go
type Store interface {
    List(context.Context) ([]scheduler.JobDefinition, error)
}
```

业务注册稳定 handler 名：

```go
registry := scheduler.NewHandlerRegistry()

if err := registry.Register("user.sync", func(
    ctx context.Context,
    execution scheduler.Execution,
) error {
    payload := execution.Definition.Payload
    return syncUsers(ctx, payload)
}); err != nil {
    panic(err)
}

runtime, err := scheduler.New()
if err != nil {
    panic(err)
}

loader, err := scheduler.NewLoader(store, registry, runtime)
if err != nil {
    panic(err)
}

if err := loader.Run(ctx, 30*time.Second); err != nil && !errors.Is(err, context.Canceled) {
    panic(err)
}
```

`JobDefinition.Version` 是 reconcile 的变更版本：同一 ID、同一 Name 只有 Version 变化才会执行 Replace。修改 Schedule、Handler、Payload、Timeout、OverlapPolicy 等字段时必须同时递增 Version。

同步流程是：

```text
Store.List
  ↓
完整 desired-state 校验
  ↓
生成 reconcile 计划
  ↓
Replace / Remove / Add
  ↓
失败反向 rollback
  ↓
提交 loaded snapshot
```

重命名或删除任务会先释放旧 Name，再添加新任务，因此新的 ID 可以安全复用旧 Name。

### 多实例 Singleton 与运行记录

`ExecutionMode == scheduler.Singleton` 时必须向 Loader 提供 `Locker`，否则定义会在同步阶段被拒绝。框架不内置 Redis/PostgreSQL 锁实现。

```go
type Locker interface {
    TryLock(
        context.Context,
        string,
        time.Duration,
    ) (scheduler.Lock, bool, error)
}
```

Timeout > 0 时框架请求的锁租期包含额外 grace；Timeout == 0 时 Locker adapter 必须保证 lease 在 `Unlock` 前持续有效，必要时自行续租。

`RunRecorder` 可记录 `running / success / failed / skipped / timeout / canceled`。每次执行使用独立 `RunID`。Recorder 是观测能力：Recorder 失败不会阻止业务 Handler 执行，但会作为 Scheduler 执行错误返回给日志路径。

## 生命周期与并发

`lifecycle.Lifecycle`：

- 同一 Start/Started/Shutdown/Stop 阶段的并发调用等待同一个 attempt；
- 后台任务使用 mutex + task counter + completion channel 跟踪；
- `Wait` 不创建额外 waiter goroutine，也没有 `sync.WaitGroup.Add` 与 `Wait` 的时序陷阱；
- `BeginShutdown` 取消由 `Server.Go` 启动的后台任务；
- Scheduler 在 HTTP listener 之前启动，并在 shutdown 中停止和等待。

框架内部共享状态的基本原则：

1. 注册期状态与请求热路径分离；
2. slice/map 在跨 ownership 边界时复制；
3. 请求读取使用不可变快照或 `RWMutex`；
4. 不能证明线程安全的第三方对象不在框架内部并发修改。

### 外部对象 ownership

以下 API 是明确的 escape hatch 或依赖注入口：

- `Server.Engine()`
- `Server.HTTPServer()`
- `WithLogger`
- `WithValidator` / `WithBindingValidator`
- `WithConfig`
- `WithScheduler`

框架不会尝试反射深拷贝这些对象。注入完成后，调用方不得与框架并发修改它们，除非该对象自身公开保证并发安全。

`Engine()` 不应被用来绕过 gonex Registry 直接注册业务路由；否则 Registry、OpenAPI 和注册原子性将失去一致性。

## gx 文件安全

`gx` 生成器的文件系统事务执行以下约束：

- 拒绝绝对路径和 `..` 越界；
- 逐级 `Lstat`，拒绝目标路径中的 symlink；
- Write/Delete 拒绝同一路径和父子路径重叠；
- 普通 file transaction 不允许删除目录；
- DirectorySwap 拒绝相同或嵌套 target；
- file/directory transaction 在整个事务生命周期持有同一个 `os.Root`；最终 backup/install/delete/rollback 都通过 descriptor-relative `Root.Rename` / `Root.Remove*` / `Root.MkdirAll` 完成，不使用“检查路径后再按字符串路径 rename”的 publication；
- 写入失败使用 backup rollback；
- `gx dao` 的生成目录只能放生成代码，不放业务手写文件。

## 验证

根 module：

```bash
go mod tidy -diff
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

`gx`：

```bash
cd gx
go mod tidy -diff
go test ./...
go test -race ./...
go vet ./...
```

独立 module：

```bash
for module in contrib/gormlog contrib/redislog examples/basic examples/demo examples/quick-demo benchmarks/gx; do
  (cd "$module" && go mod tidy -diff && go test ./... && go vet ./...)
done
```

CI 使用同一矩阵，并额外在 macOS/Windows 验证 core + gx portability；稳定 v1 发布后还会执行 public API compatibility gate。并发、生命周期、动态 CORS、Scheduler snapshot/reconcile、Session snapshot 和 gx 文件事务都有回归测试。

## 开发原则

- v1 之前发现坏 API，直接改掉，不增加 alias、wrapper 或函数转发维持旧调用；
- 框架 core 不引入 GORM/Redis/Kafka 等业务基础设施；
- `router.Registry` 保持独立于 Gin；
- 请求热路径不做可提前到注册期的反射扫描；
- 公共契约必须有测试，竞态敏感路径必须通过 `go test -race`；
- 生成器、examples、README、AGENTS 和 skills 与代码一起维护。
