# gonex 项目审查约定

## 1. 目录职责

| 目录 | 允许的职责 | 常见违规 |
| --- | --- | --- |
| `api/<module>/<module>.go` | 模块级 Controller 接口 | 数据库 Entity、业务事务、直接查询 |
| `api/<module>/<version>` | HTTP 元数据、Req/Res、参数校验 | 数据库 Entity、业务事务、直接查询 |
| `internal/controller/<module>` | API/领域映射、Service 调用、HTTP 错误 | DAO/全局 DB/GORM、可复用业务规则 |
| `internal/service` | 由 Logic 签名生成的接口和注册入口 | 手写实现、直接编辑生成文件 |
| `internal/logic/<name>` | 单个 receiver、全部业务数据库操作、业务规则与用例编排 | 建连接、创建 DAO/Repository、API/HTTP 类型 |
| `internal/model` | 跨层/跨 Logic 复用的应用与领域结构体 | API 绑定类型、生成 Entity、重复持久化细节 |
| `internal/dao` / entity 生成目录 | gx/GORM Gen 产物 | 手写 DAO、Repository 或业务代码 |
| `internal/bootstrap/db` | 初始化、暴露并关闭唯一全局 DB | 业务查询、事务、领域规则 |
| `internal/logic/logic.go` | Logic 模块 blank import 聚合 | 手工维护与 gx 竞争 |

这是 demo 及其派生项目的固定目录契约；除非用户明确要求重构架构，不采用另一套命名或所有权规则。

## 2. API 审查

- `Req` 是结构体指针并嵌入一个 `g.Meta`；`path` 以 `/` 开头，`method` 是单一受支持方法。
- `:name` / `*name` 与 `path:"name"` 一一对应，无重复和遗漏。
- 输入来源真实且唯一；JSON 字段有 `json` 标签和正确媒体类型。
- `binding` 与 `validate` 没有把合法零值误判为缺失；PATCH 能表达字段未提供。
- `operationId`、安全声明、描述、默认值和 schema 与运行时一致。
- 响应不直接暴露 Entity、凭据、内部错误或未约定字段。

## 3. Controller 审查

- 方法签名与生成契约一致，Context 原样传递。
- 只做传输映射、Service 调用和 HTTP 错误转换。
- 未知错误返回安全 500；领域错误用稳定业务码和正确 HTTP 状态。
- 普通 JSON 不直接调用 Gin；直接响应后不再返回可编码数据或第二个错误。
- 使用 Logger/Session/HTML 时从 `ghttp.FromContext` 获取并处理可能错误。
- Controller 已实际 Bind，Middleware 层级覆盖目标路由。

## 4. Logic 与 Service 审查

- Logic 不依赖 API 或 HTTP 类型，公开方法第一参数是 `context.Context`。
- 除 bootstrap 建连/关闭和 gx 生成文件外，查询、写入、事务、Raw SQL 全部位于 Logic；Controller、
  Service、model、Middleware、Scheduler handler 没有数据库调用。
- 每个 Logic 目录只有一个主要 receiver；需要第二个业务类时已拆成独立目录和独立 Service。
- 仅真正的服务能力导出 receiver 方法，内部 helper 非导出。
- Service 生成接口与 Logic 当前签名一致；没有手改 `DO NOT EDIT`。
- 每个 Logic 目录的唯一 receiver 已生成并注册对应 Service；签名中的 import alias 没有被丢失。
- Logic 注册、聚合 blank import、应用入口导入形成完整链路。
- 启动顺序完成 `config.Init()`、数据库初始化、`dao.SetDefault` 后才接收请求；关闭仍由启动层负责。
- Logic 使用 `g.Cfg()`，数据库优先使用 `dao.Q.<table>.WithContext(ctx)`；DAO 无法表达时只使用现有
  全局 DB 的 `WithContext(ctx)`。没有 `gorm.Open`、`sql.Open`、手写 DAO/Repository、连接或 Query
  副本，Controller 也没有绕过 Service 使用这些入口。
- 跨层复用的导出结构体只在 `internal/model` 定义一次；API Req/Res 留在 API，生成 Entity 留在
  `internal/model/entity`，映射边界清楚。
- 事务、幂等、资源级授权和状态转换位于 Logic；敏感数据不进入日志。
- 错误可分类并保留链，Controller 不依赖字符串匹配。

## 5. 生成与工作区安全

- `gx ctrl`、`gx service` 写入前运行对应 `--dry-run`，计划范围与当前 module 一致；`gx dao` 不支持
  dry-run，按下一条单独审查。
- 非明确清理任务不运行 `gx ctrl --clean`。
- 数据库目标、表范围和生成目录未确认时不运行 `gx dao`。命令失败必须保留旧 DAO/Entity、`go.mod`
  和 `go.sum`；成功仍是完整替换。
- 生成 Entity 的 struct tag 必须通过 `go vet`；数据库 comment 中的引号不能破坏 `gorm` tag。
- 带 `Code generated ... DO NOT EDIT.` 的文件没有人工业务改动。
- Controller 动作实现、API 和 Logic 不会在后续生成中被覆盖。
- 工作区已有改动被保留，格式化没有机械重写无关文件。

## 6. 同步矩阵

应用项目：

| 变更 | 必须同步检查 |
| --- | --- |
| API 字段或路由 | Controller 契约、OpenAPI、HTTP 测试、README |
| Logic 方法签名 | Service 生成文件、Controller 调用、Logic 测试 |
| 新 Logic 模块 | 注册函数、聚合 blank import、应用入口 |
| 新公共业务结构体 | `internal/model`、API/Entity 映射、Service 签名、序列化测试 |
| DAO、全局 DB 或配置使用 | 数据库代码仅在 Logic、bootstrap 初始化顺序、Context、关闭路径 |
| 错误语义 | Controller 映射、业务码文档、成功/失败测试 |
| gx 命令或目录约定 | 项目 AGENTS、README、CI/脚本 |

gonex 框架仓库：

```text
核心实现与测试 → gx 模板/生成器与测试 → 相关 example → README/AGENTS/architecture → `examples/demo/.agents/skills`
```

纯内部重构可以不修改所有投影，但审查结论必须说明为何公开契约、生成结果、example 和文档不受影响。

## 7. 验证与报告

先运行项目已有的格式化、测试和静态检查命令。gonex 仓库的核心、`gx` 和每个 example 是独立
Go module，必须进入各自目录验证。

报告按严重度排序，每项包含：

```text
[严重度] 简短标题 — path/to/file.go:line
影响：用户或运行时会观察到什么。
条件：如何触发。
修复：最小、契约一致的修复方向。
```

摘要不能替代发现。若无发现，说明已检查范围、实际运行的命令、未覆盖的数据库/网络/并发风险。
