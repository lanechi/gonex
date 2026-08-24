# gonex 开发与协作规则

本文件适用于整个 gonex 仓库。它定义代码边界、完成标准和验证要求；用户文档见
`README.md`，架构设计见 `docs/architecture.md`，面向 AI 的应用开发规则见
`examples/demo/.agents/skills/`。

## 项目定位

gonex 是基于 Gin 的轻量 Go Web 框架。核心关注 HTTP Server、声明式路由、Controller、请求绑定、
统一响应、Middleware、配置、日志、安全、Session、模板、静态资源、OpenAPI 和生命周期。

数据库不是 HTTP 运行时的必选能力。仓库通过 `contrib/gormlog` 提供 GORM 日志适配，
通过独立 module `gx/` 提供数据库模型及工程代码生成；业务项目自行决定数据库、缓存和
消息系统的生命周期。

## Module 与目录边界

| 路径 | 职责 |
| --- | --- |
| `g/` | 便捷入口、`g.Meta`、命名 Server 与默认配置访问 |
| `ghttp/` | HTTP 编排层：Server、路由注册、请求处理、响应和生命周期 |
| `router/` | Controller 扫描、元数据解析、绑定契约和框架路由注册表 |
| `config/` | 配置文件、`.env`、环境变量和运行时覆盖 |
| `logging/` | 与实现解耦的结构化 Logger；Zap 是内置实现 |
| `middleware/` | Request ID、请求体限制、Host、CORS、CSRF 等底层 Middleware |
| `openapi/` | OpenAPI 文档模型、Schema 生成和 Swagger HTML |
| `session/`、`cookie/` | Session 契约与内存、签名 Cookie、Redis 存储 |
| `template/`、`static/` | 模板管理和安全的静态资源挂载 |
| `lifecycle/` | 启停 Hook、后台任务跟踪与优雅退出 |
| `contrib/` | 可选集成适配器，不得反向污染核心抽象 |
| `test/` | 面向外部使用方式的框架集成、隔离、安全和回归测试 |
| `gx/` | 独立 module；`gx` CLI 和生成器实现 |
| `examples/demo/` | `gx init` 下载的规范项目模板；只集成 PostgreSQL，并携带项目 agents/skills |
| `examples/` | 独立 module；规范模板与框架契约的可运行证明 |
| `benchmarks/gx/` | 独立 benchmark module；不属于运行时依赖 |

仓库没有 `go.work`。核心、`gx`、每个 example 和 benchmark 必须在各自 module 中验证。

## 每次修改的完成定义

任何项目更新都必须把文档、生成器和示例视为同一功能的一部分。

### 强制检查顺序

1. 修改核心实现并补充最低层单元测试或 `test/` 集成测试。
2. 检查 `gx` 是否生成或复制了受影响的 API、配置、目录或样板；受影响时同步修改生成器、
   `examples/demo/` 和生成器测试。`gx init` 的项目骨架只能来自 `examples/demo/`，不得维护第二套字符串模板。
3. 更新或新增能运行该行为的 example。生成器输出约定变化时，同步 `examples/demo/`，并检查
   `examples/quick-demo` 是否仍能证明现有项目的持续生成能力。
4. 更新受影响的全部 README。架构边界、请求链路、状态归属或扩展点变化时，同时更新
   `docs/architecture.md`。
5. 检查 `examples/demo/.agents/skills/` 中的 API、Controller、Logic/Service、`gx` 和审查规则是否仍
   准确；受影响时同步对应 skill 和 reference。
6. 只有协作规范本身变化时才修改本文件；不通过无意义改字制造“已同步”的假象。

即使某一层无需内容变化，也必须完成检查，并在提交说明或交付结果中记录：
`core`、`gx`、`examples/demo`、其它 `examples`、`docs`、`examples/demo/.agents/skills` 各自的修改或“不受影响”原因。

### 变更同步矩阵

| 变更 | 必须同步 |
| --- | --- |
| 路由、绑定、校验、响应或 Middleware | `test/`、根 README、`gx` Controller/API 模板、`basic` 或 `quick-demo`、相关 skill |
| 配置、日志、安全、Session、模板、静态资源、生命周期 | 对应集成测试、根 README、架构文档、相关 example、`gx init` 模板 |
| 公共类型、方法、默认值或错误语义 | API 注释、回归测试、README 示例、生成代码编译测试 |
| `gx` 命令、生成目录或所有权 | `gx` 测试、`gx/README.md`、`examples/quick-demo` 生成结果与说明、相关 skill |
| `gx init` 项目结构或默认依赖 | `examples/demo/`、下载/解包测试、`gx/README.md`、根 README、相关 skill |
| example 行为或命令 | example 自身测试、该目录 README、`examples/README.md` |
| 架构或模块边界 | 本文件、`docs/architecture.md`、根 README、`gx`、examples 与 skills 的边界说明 |

## Go 代码规则

### 公共 API

- 使用 Go `1.26.0` 语法和标准库习惯；所有 Go 文件提交前运行 `gofmt`。
- 导出标识符必须有准确的 GoDoc；注释解释契约、所有权和限制，不复述名称。
- 公共 API 不能暴露 Zap、Viper 或 GORM 等实现类型，除非该包本身就是明确的适配器。
- 新 API 优先通过小接口、Option 或独立组件扩展，避免持续扩大 `Server` 的职责。
- 普通错误返回 `error` 并使用 `%w` 保留错误链；仅 `Must*` 或应用启动失败允许 panic。
- `context.Context` 必须沿请求、日志、生命周期和 I/O 调用传递，不保存到长期对象中。

### 路由与请求链路

- 框架自己的 `router.Registry` 是路由事实来源；Gin 路由树只是执行后端。
- 一批 Controller 路由必须先完整扫描和校验，再写入 Gin 和 Registry；失败不能留下部分注册。
- Controller 契约保持 `func(context.Context, *Req) (*Res, error)`，请求必须是结构体指针。
- `g.Meta` 负责路由/OpenAPI 元数据。`path`、`query`、`header`、`cookie`、`form`、`file`
  进入 `FieldBinding`；JSON 由请求结构体的 `json` 标签和 Content-Type 驱动。
- 自定义 `binding` 与 `validate` Validator 必须是构造完成后只读的独立实例；请求热路径不得切换
  tag name、清理 Validator 缓存或通过 `unsafe` 修改第三方私有状态。
- 反射和标签解析应在注册阶段完成并缓存；请求热路径不得重复扫描类型。
- Middleware 顺序保持“系统级 → 应用级 → 分组/Bind → 路由级 → Controller”；所有层级的
  `nil` Middleware 必须在注册阶段拒绝。
- 已经写出响应后不得追加第二个错误包络。

### 状态、并发与生命周期

- 每个 `Server` 独立拥有路由、OpenAPI 缓存、Logger、Session、模板、配置和生命周期状态。
- 新增共享可变状态前必须证明进程级语义不可避免，并提供并发保护和跨 Server 隔离测试。
- 修改运行时设置时使用现有锁和快照模式；不要把内部 slice、map 或指针直接暴露给调用方。
- Server 启动后禁止会改变路由拓扑或执行顺序的配置。
- 同一请求的 Session 必须复用；Logout 后必须驱逐请求缓存并禁止旧 handle 再次持久化；可取消的
  存储 I/O 必须透传请求 `context.Context`。
- 启动 Hook 只有整组成功后才能提交阶段状态；`lifecycle.Lifecycle` 的同阶段并发调用等待同一
  结果且失败轮次可重试。`Server.Run*` 禁止并发运行，启动失败会完成关闭清理且该实例不再复用。
- 后台任务通过 `Server.Go` 跟踪，并响应取消；资源释放使用生命周期 Hook，确保失败启动和
  正常关闭都可回收。

### 配置、安全与日志

- 配置优先级保持：运行时 `Set` > 系统环境变量 > `.env` > 配置文件 > 默认值。
- 配置错误必须通过 `Server.Err()` 或返回值可观察，不能静默降级到不安全值。
- 默认不信任代理；CORS 开启时必须显式给出允许来源；Cookie/CSRF 的
  `SameSite=None` 必须同时启用 `Secure`。
- 静态资源必须同时通过路径边界和扩展名白名单；默认仅开放 Web 页面、脚本、样式、常见图片、
  字体、Wasm 和 Web App Manifest。扩展白名单为 nil 时使用安全默认值，显式空列表拒绝全部文件。
- 错误详情只在 debug 模式响应，release/test 模式不得泄漏内部错误和堆栈。
- 所有框架日志通过 `logging.Logger`，请求日志从 Context 获取带 Request ID 的 Logger。

### 生成代码

- 带 `Code generated ... DO NOT EDIT.` 的文件只能由 `gx` 或其底层生成器更新。
- Controller 契约文件可重生成；API 定义和 Controller 业务实现首次创建后由开发者维护，不能带
  `DO NOT EDIT` 标记。
- Service 接口由 Logic 方法签名生成；Logic 实现首次创建后由开发者维护，不能被后续生成覆盖。
- `gx dao` 管理 `internal/dao` 与 `internal/model/entity` 的生成内容。它必须先在 module 内 staging
  生成、合法化并校验，再成对替换两个目录；任一步失败都要回滚目录及 module 文件。成功执行仍会
  完整替换旧生成内容，因此禁止在其中放业务手写代码并必须先确认数据库目标。
- 生成的 Go struct tag 必须通过 `reflect.StructTag`/`go vet`；数据库注释中的引号不能破坏 tag。
- 生成器必须支持 `--dry-run` 的命令不得在 dry-run 中写文件；重复运行应保持幂等。
- `gx init` 从 GitHub 对应版本的 `examples/demo/` 下载模板；开发构建使用 `main`。下载、解包、标识替换都在
  staging 中完成，拒绝越界路径、链接和特殊文件，通过清单校验后才提交目标目录。

## 文档规则

- README 面向当前目录的使用者，命令必须能从文档声明的目录直接执行。
- 根 README 只保留入口、关键能力和常用 API；设计取舍统一写入 `docs/architecture.md`。
- 不复制会快速失真的实现细节；默认值、路径、命令和生成目录必须以代码和测试为准。
- 新增、移动或删除 README/架构文档时，必须修复所有相对链接。
- 文档中的示例代码应来自或对应可编译测试/example，避免维护第二套虚构 API。
- `examples/demo/.agents/skills` 只记录会改变 AI 决策的框架事实。公共契约、`gx` 命令或推荐分层
  变化时必须同步；每个 skill 的 `SKILL.md` 负责入口和路由，详细规则放在被入口明确引用的
  `references/`。

## 验证命令

所有 shell 命令必须以 `rtk` 开头。若默认 Go cache 不可写，使用 `rtk proxy env` 指向
`/private/tmp` 下的任务专用目录。

核心 module（仓库根目录）：

```bash
rtk proxy env GOCACHE=/private/tmp/gonex-gocache go test ./...
rtk proxy env GOCACHE=/private/tmp/gonex-gocache go test -race ./...
rtk proxy env GOCACHE=/private/tmp/gonex-gocache go vet ./...
rtk git diff --check
```

生成器 module：

```bash
cd gx
rtk proxy env GOCACHE=/private/tmp/gonex-gx-gocache go test ./...
rtk proxy env GOCACHE=/private/tmp/gonex-gx-gocache go vet ./...
```

规范 demo module：

```bash
cd examples/demo
rtk proxy env GOCACHE=/private/tmp/gonex-demo-gocache go test ./...
rtk proxy env GOCACHE=/private/tmp/gonex-demo-gocache go vet ./...
```

Examples 必须逐个验证：

```bash
cd examples/basic
rtk proxy env GOCACHE=/private/tmp/gonex-basic-gocache go test ./...

cd ../quick-demo
rtk proxy env GOCACHE=/private/tmp/gonex-quick-demo-gocache go test ./...

cd ../template-demo
rtk proxy env GOCACHE=/private/tmp/gonex-template-demo-gocache go test ./...
```

性能相关变更再进入 `benchmarks/gx/` 运行 benchmark。`staticcheck` 可用时，对核心和 `gx`
分别运行；不得因为某个可选工具未安装而跳过 `go test`、`go vet` 和 `git diff --check`。

## 工作区安全

- 开始编辑前运行 `rtk git status --short` 和相关 `rtk git diff`，保留所有用户改动。
- 不使用 `git reset --hard`、`git checkout --` 或递归删除来清理工作区。
- 不手工修改不属于当前任务的生成文件、数据库文件或 benchmark 结果。
- 修改行为时优先补失败测试，再实现；交付时列出实际执行的验证和未执行原因。
