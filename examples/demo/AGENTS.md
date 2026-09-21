# gonex-demo 开发规则

本文件适用于整个项目。项目使用 gonex、Cobra、GORM 和 PostgreSQL，依赖方向固定为：

```text
API Req/Res → Controller → gx 生成的 Service → Logic → PostgreSQL / 外部端口
```

## AI Agent 分工

`.codex/config.toml` 固定使用以下角色。主线程保留任务拆分、架构决策和最终验收责任；子 Agent
只处理已明确的边界，不替主线程扩大需求。

| Agent | 模型 | 职责 |
| --- | --- | --- |
| 主线程 | Sol | 指挥、拆任务、架构决策和最终验收 |
| `architect` | Sol | 架构分析、公共契约和复杂方案 |
| `worker` | Luna | 写代码、重构和实现功能 |
| `explorer` | Luna | 搜索代码、确认文件所有权和分析调用链 |
| `reviewer` | Sol | 只读 Code Review，检查正确性、安全和回归 |
| `tester` | Luna | 测试、lint、build 和 gx 生成一致性验证 |

复杂改动先由 `explorer` 定位、必要时交给 `architect` 形成方案，再由 `worker` 实现；`tester`
提供验证证据，`reviewer` 在交付前独立审查，最后仍由主线程修复问题并验收。

## 项目 skills 路由

`.agents/skills/` 是本项目唯一的应用开发 skills 目录，允许隐式调用。开始任务前按意图加载最小集合：

| 任务 | 必须使用的 skill |
| --- | --- |
| 新增完整资源、CRUD、纵向业务切片 | `$gonex-create-resource` |
| 设计或修改 `g.Meta`、Req/Res、绑定、校验、OpenAPI | `$gonex-design-api` |
| 实现 Controller、错误映射、直接响应 | `$gonex-implement-controller` |
| 实现 Logic、数据库业务操作、生成 Service | `$gonex-implement-service`；涉及数据库时同时使用 `$gonex-use-dao` |
| 初始化或关闭数据库、Redis 等全局客户端 | `$gonex-use-data`；涉及配置时同时使用 `$gonex-use-config` |
| 生成 DAO/Entity、在 Logic 中使用 `dao.Q` 或全局 DB | `$gonex-use-dao` |
| 日志、HTML 模板 | `$gonex-use-logging`、`$gonex-use-template` |
| 代码审查或分层一致性检查 | `$gonex-review-project` |

不要用通用 Go 分层习惯覆盖这些项目 skill。完整资源 skill 会路由到专项参考；只改一层时使用对应
专项 skill，避免加载无关流程。

## 代码边界

- `api/<module>/<module>.go` 提供模块级 API 包；`api/<module>/<version>` 定义 `g.Meta`、请求参数、校验和公开响应，不暴露数据库 Entity。
- Controller 只完成 API/领域映射、调用 Service 和转换 HTTP 错误，不直接访问 GORM。
- 每个 `internal/logic/<name>` 目录只有一个主要 Logic receiver；跨层公共结构体统一放
  `internal/model`，API Req/Res 和生成 Entity 不作为公共业务模型。
- 除 bootstrap 初始化/关闭和 gx 生成 DAO 外，查询、写入、事务、Raw SQL 全部写在 Logic。优先使用
  `dao.Q`，无法表达时使用已有全局 DB；禁止自行创建 DAO、Repository、DB wrapper 或连接。
- Logic 持有业务规则、事务和数据访问编排，不依赖 API Req/Res、Gin 或 `ghttp.Context`。
- Service 接口与 `internal/logic/logic.go` 由 `gx service` 维护。带
  `Code generated ... DO NOT EDIT.` 的文件禁止手改。
- 配置统一读取 `g.Cfg()`；数据库、配置和 Logger 使用项目现有全局入口，不在业务结构体缓存副本。
- 所有请求和 I/O 必须透传 `context.Context`；敏感配置、密码、Token 和完整请求体不得写入日志。
- PostgreSQL 只从 `.env` 或系统环境变量的 `DATABASE_*` 读取；Web 配置不保存数据库凭据。
- 数据库由应用初始化，并通过 `server.OnStop` 关闭；核心 gonex Server 不拥有业务数据库。

## gx 工作流

若本地存在 `gx`，修改 API 或 Logic 后先预览再生成：

```bash
gx ctrl --dry-run
gx service --dry-run
gx ctrl
gx service
```

API、Controller 动作实现和 Logic 是开发者文件；Controller 契约、Service 和 Logic 聚合文件是生成器
文件。未经明确确认不运行 `gx ctrl --clean` 或 `gx dao`。

## 每次修改的完成定义

行为、路由、参数、目录或命令变化时，同步检查代码、生成文件、测试、README、AGENTS 和
`.agents/skills/`（本项目唯一 skills 目录）。Go 文件运行 `gofmt`，并从项目根目录执行：

```bash
go test ./...
go vet ./...
git diff --check
```

保留工作区已有改动，不使用 reset/checkout 清理用户文件。
