# gonex

gonex 是一个基于 Gin 的轻量 Go Web 框架，设计思路来源于 GoFrame v2，但只聚焦 Web Server：声明式路由、Controller、请求绑定、统一响应、Middleware、配置、日志、安全、Session、模板、静态资源、OpenAPI、生命周期和定时任务。

数据库、缓存、消息队列不是核心运行时依赖。业务项目自行持有这些基础设施的 client 与生命周期。`contrib/gormlog` 与 `contrib/redislog` 各自是独立 Go module，因此 GORM/Redis 不进入 core module 的依赖图；代码生成器 `gx` 也是独立 Go module。

> **当前处于 v1 之前。** 框架优先保证 API 简洁、语义正确、安全和并发行为可证明，不为了兼容错误或冗余 API 增加 alias、函数转发或兼容层。破坏式修改必须同步测试、示例和文档。

## 要求

- Go 1.26.0+

## 安装

```bash
go get github.com/lanechi/gonex
```

最小示例：

```go
package main

import (
    "context"

    "github.com/lanechi/gonex/ghttp"
)

type HelloReq struct {
    ghttp.Meta `path:"/hello" method:"GET" summary:"Hello"`
    Name string `query:"name"`
}

type HelloRes struct {
    Message string `json:"message"`
}

type HelloController struct{}

func (*HelloController) Hello(ctx context.Context, req *HelloReq) (*HelloRes, error) {
    return &HelloRes{Message: "hello " + req.Name}, nil
}

func main() {
    server := ghttp.NewServer()
    server.MustBind(&HelloController{})
    if err := server.Run(":8000"); err != nil {
        panic(err)
    }
}
```

访问：

```text
GET http://127.0.0.1:8000/hello?name=gonex
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
core: vX.Y.Z
gx:   gx/vX.Y.Z
```

## 能力

- Gin-backed HTTP Server；
- Controller 扫描、声明式路由、request binding；
- route registry 作为框架路由事实源；
- response encoder 与 error handler；
- global/group/route middleware；
- Viper 配置；
- Logger 抽象与 Zap 默认实现；
- Host/CORS/CSRF/Body limit 等安全中间件；
- Memory/Cookie Session；
- 模板与静态资源；
- OpenAPI/Swagger；
- lifecycle hooks、background tasks、优雅退出/重启；
- 每个 Server 独立 Scheduler；
- 持久任务 Loader、HandlerRegistry、Locker、Recorder；
- `gx controller` / `gx service` / `gx dao`。

## 配置优先级

```text
runtime config.Set > 系统环境变量 > .env > 配置文件 > 默认值
```

## OpenAPI

默认端点：

```text
GET /openapi.json
GET /docs/
```

OpenAPI `info.title` 默认使用 Server name；`info.version` 默认是 `unversioned`。应用可通过 `server.openapi.title` 和 `server.openapi.version` 显式声明文档身份。未单独声明的应用错误状态由 OpenAPI `default` response 覆盖。

## Controller、Router 与 Binder

Controller 路由方法使用：

```go
func (controller *UserController) Create(
    ctx context.Context,
    request *CreateUserReq,
) (*CreateUserRes, error)
```

请求 struct 的匿名字段必须是**精确的** `ghttp.Meta` / `router.Meta` 类型。框架不接受“字段看起来一样”的其它 struct 作为 route metadata。

路由处理链：

```text
Controller
  ↓
router.ScanController
  ↓
RouteMetadata ──→ Registry / OpenAPI
  ↓
RouteRuntime
  ↓
private Binder
  ↓
Gin execution tree
```

`RouteMetadata` 是可公开检查、可复制的声明信息；编译后的 Binder 是内部执行对象，不暴露兼容快照字段。

Registry 是框架拥有的 route source-of-truth；Gin tree 只是执行后端。批量注册在 mutation 前完成完整 preflight，避免 registry 与 Gin 半注册。

## Middleware

```go
server.Use(globalMiddleware)
server.Group("/api", groupMiddleware)
server.Route("GET", "/users/:id").Use(routeMiddleware)
```

动态 route middleware 通过框架 dispatcher 执行；启动后不修改 Gin route tree。

## Response / Error

默认成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

handler 返回值可以是任意 JSON 可编码值。

可以替换：

```go
ghttp.WithResponseEncoder(...)
ghttp.WithErrorHandler(...)
```

自定义 response/error handler 后，默认 OpenAPI 文档会关闭；如果仍需要文档，应用必须再显式启用并确保契约与自定义输出一致。

## 配置与外部对象 ownership

框架在公开 slice/map 配置边界复制数据，例如：

- CORS origins/headers/methods；
- Trusted proxies；
- Allowed hosts；
- Scheduler middleware/job metadata；
- Session canonical values。

对于通过 `Config.Set(any)` 或 Option 注入的任意外部对象，框架不做反射型“万能深拷贝”。Logger、Validator、Config、custom Engine 等对象由调用方拥有；如果调用方要在多个 goroutine 同时修改这些对象，对象本身必须提供并发安全保证。

## 安全默认值

- Gin 默认 trusted proxies 被关闭，只有显式配置的 proxies 才受信任；
- `SameSite=None` 的 Session/CSRF Cookie 必须为 Secure；
- Body、multipart memory、Header 都有默认上限；
- release/test 模式不向客户端返回内部错误详情；
- 静态资源有路径边界、symlink 和扩展名约束；静态挂载属于 Gin topology mutation，必须在 `Run*` 前完成，并与启动通过 `registrationMu` 序列化；
- 运行期 Host/CORS/CSRF 设置通过快照与锁切换，不把调用方 slice 直接暴露给请求热路径。

## Session

Session value 进入框架时会转成 JSON-safe detached canonical value：

```text
nil
bool
string
json.Number
[]any
map[string]any
```

因此：

- `Set` 后修改原 map/slice 不会反向修改 session；
- `Get` 返回 detached snapshot；
- Cookie 与 Memory storage 对数字采用 `json.Number`，避免大整数被 float64 截断；
- func、channel、cycle 等不可 JSON 编码值返回错误；
- 不承诺保留任意 Go concrete type。

## Scheduler

Server 内置 Scheduler：

```go
scheduler := server.Scheduler()

_ = scheduler.Add(schedulerpkg.Job{
    Name:     "cleanup",
    Schedule: schedulerpkg.Every{Duration: 5 * time.Minute},
    Handler: func(ctx context.Context) error {
        return nil
    },
})
```

核心接口：

```go
type Scheduler interface {
    Start(context.Context) error
    Stop()
    Wait(context.Context) error
    Add(Job) error
    Remove(name string) error
    Jobs() []JobInfo
    Use(...Middleware) error
}

type MutableScheduler interface {
    Scheduler
    Validate(Job) error
    Replace(Job) error
}
```

`Replace` 用于持久任务 reconcile：验证和新 runtime 安装成功后才提交内存状态，并在 replacement 之间复用 overlap gate。

### Overlap policy

```text
SkipIfRunning  默认；当前任务未结束时跳过新 trigger
AllowOverlap   允许重叠执行
QueueOne       当前任务运行中时最多保留一个补跑
```

## 持久任务

业务系统负责数据库 schema、查询与 adapter；framework 只定义 contract。

```go
type Store interface {
    List(context.Context) ([]JobDefinition, error)
}
```

`JobDefinition` 保存稳定 `Handler` ID，而不是 Go 函数路径：

```text
DB JobDefinition
   ↓ Handler ID
HandlerRegistry
   ↓ resolve
scheduler.Job
```

`Version` 是 reconcile 的 authoritative change token：运行定义发生变化时，数据库 writer 必须递增它。相同 ID + Name + Version 不执行 Replace。

Loader：

- 每轮先读取完整 desired state；
- 全量 validate；
- stale/rename 先移除；
- Add/Replace/Remove 失败时执行逆操作 rollback；
- 只有完整成功才提交 `loaded` snapshot；
- 不存在 reflection deep-equal fallback。

### Singleton

持久任务支持：

```go
ExecutionLocal
ExecutionSingleton
```

Singleton 锁键：

```text
scheduler:job:<stable JobDefinition.ID>
```

`Timeout > 0` 时框架请求的锁租期包含额外 grace；`Timeout == 0` 时 Locker adapter 必须保持 lease 有效直到 `Unlock`。

Recorder 的 Start/Finish 与 Unlock cleanup 使用独立的 bounded background context，调用方 canceled context 不会直接跳过审计结束记录和锁释放。

## Lifecycle

生命周期：

```text
OnStart
  ↓
OnStarted
  ↓
Running
  ↓
OnShutdown
  ↓
tracked tasks drain
  ↓
OnStop
```

同一 phase 的并发调用共享 attempt/result；tracked task 使用 count + channel barrier，不在 Wait 期间调用 `WaitGroup.Add`。

## Template

Template Manager 的 root/functions/cache/watcher 由内部 RWMutex 管理。`SetRoot`、`AddFunc`、`Reload`、`Execute` 和 watcher reload 可以安全协调，不需要把模板 root 强制限制为 startup-only。

## gx 文件事务

`gx` 的 generator 输出统一经过 staging transaction：

- 输入 path 必须是 project-relative；
- path component 是 symlink 时拒绝；
- Write/Delete 拒绝同一路径和父子路径重叠；
- 普通 file transaction 不允许删除目录；
- DirectorySwap 拒绝相同或嵌套 target；
- file/directory transaction 在整个事务生命周期持有同一个 `os.Root`；最终 backup/install/delete/rollback 都通过 descriptor-relative `Root.Rename` / `Root.Remove*` / `Root.MkdirAll` 完成，不使用“检查路径后再按字符串路径 rename”的 publication；
- 写入失败使用 backup rollback；
- `gx dao` 的生成目录只能放生成代码，不放业务手写文件；
- DAO 目录 publication 一旦 `Commit` 完成，即使之后只有 backup/root-handle cleanup 报错，也不会把新的 `go.mod/go.sum` 回滚成旧版本；只有尚可逆的事务才同时回滚目录与 module 文件。

## 开发验证

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

- Registry 是路由事实源，Gin 是执行后端；
- 核心接口围绕 HTTP runtime，不耦合数据库和 MQ；
- scheduler persistence 存稳定 handler ID，不存 Go function reference；
- Session 存 JSON-safe value，不承诺 arbitrary concrete type preservation；
- pre-v1 遇到错误 API 直接删除或重设计，不通过 alias/wrapper/forwarder 保留旧调用；
- 并发边界必须明确：immutable snapshot、锁或显式 caller ownership 三选一；
- 错误路径必须考虑 rollback 和资源释放；
- 修改完成后同步 README、AGENTS、examples，并运行完整 CI。