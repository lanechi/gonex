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

## 代码边界

- `api/<module>/<module>.go` 提供模块级 API 包；`api/<module>/<version>` 定义 `g.Meta`、请求参数、校验和公开响应，不暴露数据库 Entity。
- Controller 只完成 API/领域映射、调用 Service 和转换 HTTP 错误，不直接访问 GORM。
- Logic 持有业务规则、事务和数据访问编排，不依赖 API Req/Res、Gin 或 `ghttp.Context`。
- Service 接口与 `internal/logic/logic.go` 由 `gx service` 维护。带
  `Code generated ... DO NOT EDIT.` 的文件禁止手改。
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
