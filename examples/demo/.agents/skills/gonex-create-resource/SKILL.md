---
name: gonex-create-resource
description: 为 gonex 项目创建完整 API 资源，并协调 API、Controller、Logic、Service、测试和 gx 生成结果；适用于新增端点、CRUD 资源或纵向业务切片。
---

# 创建 gonex 资源

新增或扩展一个资源时，先阅读 [references/workflow.md](references/workflow.md)，再修改代码。

## 开始前

1. 读取项目根目录及目标目录中的 `AGENTS.md`，检查 `go.mod`、现有 `api/`、
   `internal/controller/`、`internal/logic/`、`internal/service/` 和测试布局。
2. 复用项目已有的模块名、版本、错误码、模型、分页和注册方式，不凭空引入第二套分层。
3. 用 `command -v gx` 探测本地生成器。存在时先查看目标子命令的 `--help`，每次写入前先运行
   对应的 `--dry-run`。不存在时按现有目录手工实现；未经用户要求不要联网安装工具。

## 资源闭环

按依赖顺序维护以下契约：

```text
API Req/Res → Controller 契约与实现 → Service 接口 → Logic 实现 → 启动注册 → 测试与文档
```

- API 类型声明 HTTP 契约；不要让 Logic 接收 API Req/Res。
- Controller 只映射 API/领域模型、调用 Service、转换 HTTP 错误。
- Logic 持有业务规则和数据访问编排；Service 是由 Logic 导出方法生成的稳定调用边界。
- 启动入口必须 blank-import `internal/logic` 聚合包，确保 Logic 的 `init` 注册执行。
- 修改请求或 Logic 方法后，重新预览并同步 `gx ctrl`、`gx service` 的输出。

## 所有权与安全

- 看到 `Code generated ... DO NOT EDIT.` 时停止手工编辑，改源 API/Logic 或运行相应生成命令。
- 只编辑明确由开发者维护的 API、Controller 动作实现、Logic、业务模型和测试。
- 未经明确要求不运行 `gx ctrl --clean`；数据库目标和影响范围未确认时不运行 `gx dao`。
- 保留工作区已有改动。生成计划若包含意外覆盖或删除，停止并说明原因。

## 完成标准

至少验证目标 module 的格式化、编译和测试；检查路由已绑定、Service 已注册、OpenAPI 与真实参数
一致。行为或命令发生变化时，同步项目 README/AGENTS；修改 gonex 框架本身时还必须同步
`gx`、相关 `examples`、测试和架构文档。
