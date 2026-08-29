# gonex 开发与协作规则

本文件适用于整个 `lanechi/gonex` 仓库，面向人和代码 Agent。用户文档见 `README.md`，架构说明见 `docs/architecture.md`，应用项目 skills 见 `examples/demo/.agents/skills/`。

## 1. 项目定位

gonex 是基于 Gin 的轻量 Go Web 框架，只负责 Web Server 相关能力：HTTP、声明式路由、Controller、Binder、Middleware、配置、日志、安全、Session、模板、静态资源、OpenAPI、生命周期和 Scheduler。

数据库、缓存、消息队列属于业务基础设施，不进入 core。`contrib/gormlog` 与 `contrib/redislog` 分别是独立 Go module，不能把 GORM/Redis 依赖重新带回 root module；`gx/` 是独立代码生成 module。

## 2. v1 前的 API 原则：不做兼容层

当前项目处于 v1 之前，**正确性优先于兼容性**。

发现公共 API 设计错误、重复、泄漏内部状态或阻碍安全并发时：

- 直接删除或重设计；
- 不创建旧名称 alias；
- 不创建只负责转发的 wrapper/function；
- 不保留 compatibility field/snapshot；
- 不为了旧调用维护双路径；
- 不维护全量 public API golden 来阻止有意的 breaking cleanup。

禁止示例：

```go
// 禁止：为了旧 API 继续存在而转发。
func OldBind(...) error { return NewBind(...) }

// 禁止：为了兼容暴露内部运行态副本。
type Binder struct {
    Fields []FieldBinding
    fields []FieldBinding
}
```

可以保留针对**已明确删除的坏 API**的回归测试，防止 Agent 以后重新引入兼容垫片。

每个 breaking change 必须同步：调用点、测试、examples、生成器（若受影响）和文档。

## 3. Module 与目录边界

| 路径 | 职责 |
| --- | --- |
| `g/` | `g.Meta`、命名 Server、默认配置便捷入口 |
| `ghttp/` | Server、路由编排、请求处理、响应、安全、Session HTTP 集成、生命周期托管 |
| `router/` | Controller 扫描、RouteMetadata、Binder 编译、Registry |
| `config/` | 配置文件、`.env`、环境变量、运行时覆盖 |
| `logging/` | Logger 抽象与默认实现 |
| `middleware/` | 独立 HTTP Middleware 能力 |
| `openapi/` | OpenAPI/Schema/Swagger |
| `session/` | Storage、MemoryStorage、CookieStorage |
| `template/` | 模板解析、缓存、watcher |
| `static/` | 安全静态资源挂载 |
| `lifecycle/` | Hook、后台任务、阶段同步 |
| `scheduler/` | 本地 Scheduler、持久任务 Loader、Locker、Recorder |
| `internal/` | core 内部共享实现，不形成公共 API |
| `contrib/` | 独立 Go module 的可选第三方适配，不反向污染 core module graph |
| `test/` | 外部契约、架构、安全、集成回归 |
| `gx/` | 独立 module 的 CLI/生成器 |
| `examples/demo/` | `gx init` 的规范项目模板 |
| `examples/*` | 独立 module 的可运行证明 |
| `benchmarks/gx/` | 独立 benchmark module |

仓库没有根级 `go.work`。不同 module 必须分别验证。

## 4. 完成定义

一次修改只有满足以下条件才算完成：

1. 生产代码已完成；
2. 最低层单元测试或集成测试覆盖新契约；
3. 并发敏感路径有 race 测试或可被现有 `-race` 测试实际触达；
4. `gx` 若生成受影响 API/目录，已同步；
5. `examples/demo` 与其它受影响 example 已同步；
6. README/架构/skills 中受影响事实已同步；
7. 根 module、gx module、contrib modules 和其它独立 modules 完成 `go mod tidy -diff`、test/vet；并发敏感的 root/gx 还必须通过 `-race`；
8. `git diff --check` 通过；
9. 最后再做一次源码审查，检查竞态、ownership、rollback、错误路径和资源释放。

不要把“CI 绿”当作源码审查的替代品。

## 5. Go 基础规则

- 使用 Go `1.26.0`；
- 所有 Go 文件必须 `gofmt`；
- 导出 API 必须有准确 GoDoc；
- 错误用 `%w` / `errors.Join` 保留链；
- 仅 `Must*` 或应用启动不可恢复失败使用 panic；
- `context.Context` 沿请求/I/O/任务传递，不长期保存请求 Context；
- core 公共 API 不泄漏 Zap、Viper、GORM、gocron 等实现类型；
- 新能力优先小接口和独立 package，不继续膨胀 `ghttp.Server`；
- 不使用 `unsafe` 绕过第三方并发或缓存规则。

## 6. Ownership 与并发总原则

所有共享可变状态必须能回答两个问题：

1. 谁拥有它？
2. 谁用什么同步方式修改它？

默认规则：

- 框架接收自己声明的 slice/map 配置时，在跨 ownership 边界复制；
- 请求热路径只读 immutable snapshot，或通过明确的 `RWMutex`；
- 不在锁外继续读取会被其它 goroutine 修改的 struct pointer 字段；
- 不返回内部 map/slice 让调用方直接修改；
- 不在持锁状态执行可能长时间阻塞的用户代码或外部 I/O，除非原子语义确实要求且已有边界设计；
- 多把锁必须保持统一顺序；当前 Server 注册/运行相关路径使用 `registrationMu` → `stateMu`；
- 关闭路径必须有有界 Context，不能永久卡在外部 recorder/locker/storage；
- 新增 goroutine 必须有明确退出路径，不允许“fire-and-forget”资源泄漏。

### 外部注入对象

以下属于依赖注入或 escape hatch：

- `WithLogger`
- `WithValidator` / `WithBindingValidator`
- `WithConfig`
- `WithScheduler`
- `WithEngine`
- `Server.Engine()`
- `Server.HTTPServer()`

框架不反射深拷贝任意第三方对象。调用方在注入后不得与框架并发修改它们，除非该对象自身保证线程安全。

`Engine()` 不得用于绕过 gonex 注册体系直接添加业务路由；否则 Registry、OpenAPI 和原子注册不再一致。

## 7. Router / Controller / Binder

### 7.1 Meta

`g.Meta` 必须保持：

```go
type Meta = router.Meta
```

扫描时使用精确类型身份。不要接受“别的 package 里也叫 Meta”的结构体。

### 7.2 RouteMetadata 与 Runtime 分离

`router.Registry` 只保存文档/检查所需的 `RouteMetadata`，不得持有 Controller reflect runtime、Binder 执行状态或 Gin handler。

运行时 reflect method/Binder 属于 `RouteRuntime`，生命周期只到注册后的执行闭包。

### 7.3 Binder

Binder 是注册期编译出的私有执行计划：

- 不恢复 `Binder.Fields` compatibility snapshot；
- 不在 Binder 上保存 Server 的可变 multipart limit；
- multipart limit 在请求执行时显式传入；
- 请求热路径不得重新扫描 struct tag；
- 外部检查绑定元数据使用 `RouteMetadata.Bindings`。

### 7.4 原子注册

Controller 批次必须先完成全部校验，再修改真实 Gin 路由树：

```text
Scan Controller
→ Compile Binder
→ Validate path bindings
→ Validate middleware/handler count
→ Registry.Validate
→ temporary Gin tree validation
→ real Gin registration
→ Registry commit
```

禁止出现“前几个 route 已写入，后一个失败”的可观察半状态。

`router.Registry` 必须继续独立于 Gin。

## 8. ghttp Server

- 每个 Server 独立拥有 Registry、OpenAPI cache、security settings、SessionManager、templates、lifecycle、scheduler；
- `Run*` 不允许同一个 Server 并发启动；
- address、trusted proxies、route topology 等启动相关修改必须和启动序列化；
- 任何会修改 Gin topology 的入口（包括 Static/StaticFile/StaticFS）必须先持有 `registrationMu`，并在同一临界区检查 running；禁止 check-then-lock；
- 动态 Host/CORS/CSRF 使用 `settingsMu` + immutable handler/snapshot 切换；
- CORS slice 必须脱离调用方所有权后才能进入请求热路径；
- OpenAPI cache 使用独立锁，路由变更时显式 invalidate；
- 写出 HTTP response 后不能再追加第二个 error envelope；
- `Server.Err()` 的初始化错误必须可观察，不静默降级。

`SetTemplateRoot`/Template Manager 可以运行期使用，因为 template package 自身锁住 root/cache/watcher；不要在 ghttp 外层再建立重复锁模型。

## 9. Validator

`binding` 与 `validate` 使用独立 Validator 实例。

自定义 Validator：

- 必须在构造 Server 前配置完成；
- 注入后视为只读；
- 请求热路径不调用 `SetTagName`、不清缓存、不修改注册规则；
- `WithBindingValidator` 与 `WithValidator` 不得使用同一个实例。

## 10. Session

Session 值采用 **JSON-safe canonical detached value** 契约。

### 10.1 允许值

必须能由 `encoding/json` 表达。进入 Session 后规范化成：

- `nil`
- `bool`
- `string`
- `json.Number`
- `[]any`
- `map[string]any`

struct、typed slice/map 等可以作为输入，但存储后不承诺保持原 Go concrete type。

### 10.2 禁止 alias

- `managedSession.Set` 必须先 normalize，不保留调用方 map/slice；
- `managedSession.Get` 返回 clone；
- `MemoryStorage.Set/Get` 必须 detach；
- `CookieStorage` 解码使用 `UseNumber`；
- 自定义 Storage 返回值进入 managed session 前必须 normalize；
- 不恢复旧的 reflect “best-effort clone arbitrary object”实现。

func、chan、循环引用、NaN/Inf 等返回错误。

### 10.3 存储与生命周期

`session.Storage` 保持 context-first：

```go
type Storage interface {
    Get(context.Context, string) (map[string]any, error)
    Set(context.Context, string, map[string]any, time.Duration) error
    Delete(context.Context, string) error
}
```

Redis 等外部 client 由业务持有，core 不提供 Redis Session client/driver。

Logout 必须：

- 删除/撤销持久状态；
- 删除 Cookie；
- 清空旧 handle；
- 标记 loggedOut；
- 驱逐同请求 Session cache。

## 11. Lifecycle

`lifecycle.Lifecycle` 的目标是阶段幂等、并发调用共享结果、后台任务可取消。

- Start / Started / Shutdown / Stop 使用 phase attempt；
- 同阶段并发调用等待同一个 `done`；
- 后台任务使用 `taskCount + taskDone channel`；
- 不重新使用 `sync.WaitGroup` 构造 Add/Wait 时序限制；
- `Wait` 不为每次等待创建辅助 goroutine；
- `BeginShutdown` 一次性取消 task context；
- Stop 在 Shutdown 后执行；
- Hook 不得长期阻塞且必须尊重 Context。

如修改阶段状态机，必须添加高并发回归测试并用 `go test -race` 验证。

## 12. Scheduler

### 12.1 基础 Scheduler

公共接口不泄漏 gocron 类型。

支持：Cron / Every / Once、Timeout、RunImmediately、Middleware、OverlapPolicy。

`Once + RunImmediately` 必须拒绝。

Overlap：

- `SkipIfRunning` 默认；
- `AllowOverlap`；
- `QueueOne` 最多保留一个待运行触发。

### 12.2 MutableScheduler

持久 reconcile 只能接受：

```go
type MutableScheduler interface {
    Scheduler
    Validate(Job) error
    Replace(Job) error
}
```

不要恢复“检测不到 Replace 就 Remove+Add”的兼容 fallback。

`Replace` 必须：

- 失败时保留旧任务；
- 跨版本复用同一个 overlap gate；
- 新 engine job 安装并确认旧 job 移除后再切换 overlap policy；
- 不产生 `SkipIfRunning` 跨版本失效窗口。

`Jobs()` 必须在 manager lock 内复制所有 mutable record 字段，再到锁外查询 engine handle；禁止复制 `*jobRecord` 后锁外读取其可变字段。

### 12.3 Persistent Loader

Store 保持数据库无关：

```go
type Store interface {
    List(context.Context) ([]JobDefinition, error)
}
```

不要重新加入未使用的 `Store.Get`。

`Loader.Sync` 必须遵循：

```text
load desired state
→ normalize + validate all definitions
→ lock loader reconcile state
→ build/apply mutations
→ rollback all applied operations on failure
→ commit loaded snapshot only after success
```

约束：

- duplicate ID/name 拒绝；
- `Enabled=false` 不进入 runtime；
- Handler 必须提前注册；
- invalid ExecutionMode 拒绝；
- Singleton 没 Locker 拒绝；
- Same ID + same Name + Version changed 使用 Replace；
- rename/stale 先 Remove，再 Add，允许新 ID 复用旧 Name；
- Payload 每次 load/run deep-copy；
- ctx cancel 在 mutation 阶段也必须检查。

**Version 是 reconcile 版本。** 修改 Handler/Schedule/Payload/Timeout/OverlapPolicy/ExecutionMode 等持久定义时必须递增 Version。不要偷偷做昂贵 deep-equal 来替代版本契约。

### 12.4 Locker

`ExecutionMode == Singleton` 必须使用 Locker。

- lock key 使用稳定 job ID，不用可重命名 Name；
- acquired=true 但 Lock 为 nil 必须报错；
- Locker panic 转成错误；
- Timeout > 0 的 lease 包含 grace；
- Timeout == 0 表示 adapter 必须保证锁在 Unlock 前持续有效，必要时自行续租；
- Unlock 使用有界 cleanup Context。

### 12.5 RunRecorder

每次运行有独立 RunID。

状态：

```text
running
success
failed
skipped
timeout
canceled
```

Recorder 是 observability，不得因为 `Start` 失败而跳过业务 Handler。Recorder/Unlock cleanup 必须有界，并捕获 adapter panic。最终记录必须在 unlock 结果已知后写入，避免先记录 success 再发生 unlock failure。

## 13. 配置、日志和安全

配置优先级固定：

```text
runtime Set > process env > .env > config file > default
```

`ViperConfig` 自身的方法通过锁保护，但 `Set(any)` 可接收外部对象：调用方不得在 Set 后并发修改同一个自定义 pointer/map/slice，除非自行同步。不要实现“反射深拷贝任意 Go 对象”制造虚假安全保证。

安全要求：

- 默认不信任代理；
- CORS 必须显式来源；
- credentials + wildcard origin 禁止；
- SameSite=None 必须 Secure；
- Body/multipart/Header 有上限；
- release/test 不泄漏内部错误；
- static 同时校验 URL/path、root boundary、symlink 和扩展名。

所有框架日志走 `logging.Logger`。

## 14. gx 文件系统安全

`gx/internal/gen/fs` 是生成器的安全边界。

普通 Transaction：

- root-relative only；
- 拒绝绝对路径和 `..`；
- 对现有路径逐级 `Lstat`；
- 拒绝 symlink component；
- Write 拒绝现有 directory；
- Delete 拒绝 directory；
- Write/Delete 之间以及同类操作之间拒绝 equality/ancestor/descendant overlap；
- Transaction 从创建到 Commit/Rollback 持有同一个 `os.Root`；最终 publication/backup/delete/rollback 必须使用 descriptor-relative `Root.Rename` / `Root.Remove*` / `Root.MkdirAll`；
- 禁止恢复“先 `safeProjectPath` 校验，再用 process-visible 字符串路径 `os.Rename`”的 TOCTOU publication；
- 每个成功创建的 Transaction 必须在所有路径 Commit 或 Rollback，确保 Windows 等平台及时释放 root handle；
- 出错 rollback backup。

DirectoryTransaction：

- stage 必须是 project root 内的 absolute、existing、non-symlink directory，并转换成同一 `os.Root` 下的 relative path；
- Target 必须 project-relative；
- 多 Target 不能相同或父子嵌套；
- install 失败恢复旧目录；
- `gx dao` 的 generated directory 不允许手写业务代码。

不要为了方便削弱这些检查。

## 15. 生成器规则

- `Code generated ... DO NOT EDIT.` 文件只能由 gx 更新；
- 用户业务实现不能因重新生成被覆盖；
- `gx init` 唯一项目模板来源是 `examples/demo/`，不维护第二套字符串模板；
- 下载、解包、module/name 替换必须在 staging 完成；
- archive 拒绝 traversal、symlink、特殊文件；
- dry-run 不写文件；
- 生成命令重复运行应有确定结果；
- struct tag 必须经 go/parser/reflect/go vet 验证；
- 涉及数据库生成目录的操作必须事务式替换。

## 16. 文档与 examples

代码事实优先级：

```text
production code + tests > examples > README/AGENTS/skills
```

文档不能描述不存在的 API。

公共 API、默认值、目录结构、Scheduler/Session 语义变化时检查：

- 根 README；
- `docs/architecture.md`；
- `gx/README.md`；
- `examples/README.md`；
- `examples/demo/README.md`；
- `examples/demo/.agents/skills/`。

只修改真正受影响的文档，不为了“同步”制造无意义 diff。

## 17. 强制验证矩阵

### Core

```bash
go mod tidy -diff
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

### gx

```bash
cd gx
go mod tidy -diff
go test ./...
go test -race ./...
go vet ./...
```

### 独立 modules

```bash
for module in contrib/gormlog contrib/redislog examples/basic examples/demo examples/quick-demo benchmarks/gx; do
  (cd "$module" && go mod tidy -diff && go test ./... && go vet ./...)
done
```

修改并发、Scheduler、Lifecycle、Session、文件事务、安全 Middleware 时，**必须关注 `-race` 结果，不得只跑普通 test**。

## 18. 最终审查清单

提交前逐项回答：

- 是否新增了不必要的 public API？
- 是否为了兼容保留 alias/wrapper/forwarder？
- 是否有调用方 slice/map/pointer 穿透进入框架共享状态？
- 是否在锁外读取其它 goroutine 会写的字段？
- 是否存在锁顺序反转？
- 是否在持锁期间调用不可控用户代码？
- shutdown/rollback 是否会因外部 adapter 永久阻塞？
- 失败后是否留下半注册、半替换或 orphan runtime state？
- Scheduler Replace 是否跨版本保留 overlap 语义？
- Session Get/Set 是否保持 detached snapshot？
- gx publication 是否始终绑定 retained `os.Root`，且每个 transaction 都在所有路径关闭？
- root/gx/modules 的 tidy、test、race、vet 与 macOS/Windows portability 是否全部通过？
- README、AGENTS、examples、skills 是否仍描述当前代码？

只要其中一项不能明确回答，就继续修，不把问题留给后续兼容层。
