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
- Memory/Cookie Session，Cookie revocation 通过显式 store 持久化；
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

环境变量和配置文件不是通过相同的嵌套结构合并，而是按照配置 key 一一对应。配置路径中的 `.` 会转换成 `_`，并统一转换为大写；驼峰也会拆成下划线。

配置命名建议遵循以下大小写规则：

- `config.yaml` 使用小写或 lowerCamelCase，例如 `server.address`、`server.maxBodyBytes`；
- `.env` 和系统环境变量使用大写下划线，例如 `SERVER_ADDRESS`、`SERVER_MAX_BODY_BYTES`；
- 使用 `Unmarshal` 时，结构体字段通过 `mapstructure` 标签声明 YAML key，例如 `` `mapstructure:"maxBodyBytes"` ``；
- Linux/macOS 的系统环境变量区分大小写，应用会按生成的大写名称查找，不要使用 `server_address` 或 `serverAddress`；
- 变量值是否区分大小写由具体配置项决定，不能一概而论。

例如 `config.yaml`：

```yaml
server:
  address: ":8000"
  maxBodyBytes: 10485760
session:
  storage:
    type: memory
```

对应的 `.env` 或系统环境变量是：

```dotenv
SERVER_ADDRESS=:9000
SERVER_MAX_BODY_BYTES=20971520
SESSION_STORAGE_TYPE=cookie
```

读取时：

```go
address := g.Cfg().GetString("server.address")
// address == ":9000"
```

环境变量只覆盖匹配的具体 key，不会把整个 `server` 或 `session` 对象替换掉。没有对应环境变量的字段继续使用配置文件值。使用 `Unmarshal` 时，目标结构体字段应通过 `mapstructure` 标签声明配置 key：

```go
var cfg struct {
  Server struct {
    Address      string `mapstructure:"address"`
    MaxBodyBytes int    `mapstructure:"maxBodyBytes"`
  } `mapstructure:"server"`
}

if err := g.Cfg().Unmarshal(&cfg); err != nil {
  // handle error
}
```

`g.Cfg()` 会懒加载项目根目录的 `.env` 和默认配置文件。默认配置文件按以下顺序查找，第一个存在的文件生效：`./config.yaml`、`./config/config.yaml`、`./manifest/config/config.yaml`。`gx dao` 是例外：它只读取 `DATABASE_*` 的系统环境变量和 `.env`，不会读取 `config.yaml`。

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
- Cookie 在 response headers 已提交后拒绝写入，避免 session token 已轮换但新 Cookie 无法发送；
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

### CookieStorage 与撤销状态

Cookie session 是客户端持有的签名 token。仅验证 HMAC 不足以实现可靠的 `Regenerate` / `Logout`，因为旧 token 在过期前仍可能被重放。因此 `CookieStorage` **必须显式提供 `CookieRevocationStore`**：

```go
revocations := session.NewMemoryCookieRevocationStore()
storage, err := session.NewCookieStorage(secret, revocations)
if err != nil {
    panic(err)
}

manager := ghttp.NewSessionManager(storage, "session_id", 24*time.Hour)
server := ghttp.NewServer(ghttp.WithSessionManager(manager))
```

`MemoryCookieRevocationStore` 只适用于测试或明确的单进程场景，进程重启后状态会消失。生产环境如果要求 logout/revocation 跨重启或多实例生效，应实现共享的 `CookieRevocationStore`，例如用 Redis/数据库事务或 Lua 保证 family token 注册与 revoke 的原子性。core 不直接依赖 Redis client。

撤销 store 接收的是 token/family 的 SHA-256 digest，不需要保存原始 Cookie 值。

如果只通过配置文件启用 CookieStorage，必须显式声明进程内撤销：

```yaml
session:
  storage:
    type: cookie
    secret: "至少 32 bytes 的高熵 secret"
    revocation: memory
```

不写 `revocation: memory` 会作为初始化错误处理，而不是静默选择进程内状态。需要共享 store 时应通过 `WithSessionManager` 注入。

Session mutation 的顺序保证：replacement cookie 先完成验证，旧 token/ID 的撤销或删除成功后才发布新 Cookie；`Logout` 的 authoritative storage/revocation 失败时保留当前 handle，以便调用方重试。

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

`Replace` 用于持久任务 reconcile。运行中的 replacement 先创建为 **unpublished generation**：即使 engine 因 `RunImmediately` 触发，也必须等到旧 engine job 清理成功、manager state 提交后才允许执行；失败则 abort 新 generation。旧 record 的 cleanup orphan 先处理，正式 old handle 最后删除，避免 cleanup 失败时把当前有效任务先移除。

跨 generation 复用同一个 overlap gate。已经被 `QueueOne` 接受的 trigger 是已承诺工作，后续 policy 切换不会把它静默清掉；policy 变化只影响未来 trigger。

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
listener bind
  ↓
HTTP Serve enters Accept
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

Lifecycle phase 不允许同步重入或并发等待自己：当 Start/Started/Shutdown/Stop 已有 phase invocation 在执行时，新的同步进入返回 `lifecycle.ErrPhaseInProgress`，而不是等待 active attempt。这样 `OnStart -> Shutdown`、`OnShutdown -> Stop`、`OnStop -> Stop` 等 hook 重入不会形成自等待死锁。

startup hook 执行期间收到 shutdown 时会立即记录 shutdown intent、取消 tracked-task context，并返回 `lifecycle.ErrStartupInProgress`；Server 让 startup 解开后再执行真正 cleanup。

tracked task 使用 `taskCount + taskDone channel` barrier，不使用会产生 Add/Wait ordering 限制的 `sync.WaitGroup`。

Graceful restart 在 replacement process 真正进入 Serve/Accept 并发出 ready 信号后完成 ownership handoff。此时即使父进程后续 cleanup 返回错误，也不会再 kill 已接管流量的 child，避免 listener 已关闭后造成双端不可用。

## Template

Template Manager 的 root/functions/cache/watcher 由内部 RWMutex 管理。`SetRoot`、`AddFunc`、`Reload`、`Execute` 和 watcher reload 可以安全协调，不需要把模板 root 强制限制为 startup-only。

## gx 文件事务

`gx` 的 generator 输出统一经过 staging transaction：

- 输入 path 必须是 project-relative；
- path component 是 symlink 时拒绝；
- Write/Delete 拒绝同一路径和父子路径重叠；
- 普通 file transaction 不允许删除目录；
- DirectorySwap 拒绝相同或嵌套 target；
- DirectorySwap 在任何 mutation 前同时拒绝 stage↔stage、stage↔target 的同路径/父子路径交叉重叠，避免备份一个 target 时把另一个 staged directory 一并搬走；
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
  (cd "$module" && go mod tidy -diff && go test ./... && go test -race ./... && go vet ./...)
done
```

CI 使用同一矩阵，并额外在 macOS/Windows 验证 core + gx portability；稳定 v1 发布后还会执行 public API compatibility gate。并发、生命周期、动态 CORS、Scheduler snapshot/reconcile/overlap、Session snapshot/revocation 和 gx 文件事务都有回归测试。

## 开发原则

- Registry 是路由事实源，Gin 是执行后端；
- 核心接口围绕 HTTP runtime，不耦合数据库和 MQ；
- scheduler persistence 存稳定 handler ID，不存 Go function reference；
- Session 存 JSON-safe value，不承诺 arbitrary concrete type preservation；
- Cookie revocation persistence 必须显式，不把 process-local blacklist 冒充多实例语义；
- pre-v1 遇到错误 API 直接删除或重设计，不通过 alias/wrapper/forwarder 保留旧调用；
- 并发边界必须明确：immutable snapshot、锁或显式 caller ownership 三选一；
- 错误路径必须考虑 rollback 和资源释放；
- 修改完成后同步 README、AGENTS、examples，并运行完整 CI。
