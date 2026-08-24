---
name: gonex-design-api
description: 设计或修改 gonex API 的 g.Meta、请求来源、校验、响应类型和 OpenAPI 契约；适用于新增端点或调整 path/query/header/cookie/JSON/form/file 参数。
---

# 设计 gonex API

涉及请求或响应模型时，先阅读 [references/parameters.md](references/parameters.md)。

## 设计步骤

1. 检查同模块、同版本 API 以及 Controller 绑定位置，确认真实路由前缀、命名和响应风格。
2. 为每个 Controller 动作定义一对明确的 `*Req`、`*Res`；`Req` 必须嵌入一个带路由元数据的
   `g.Meta`。
3. 每个输入字段只选择真实 HTTP 来源：`path`、`query`、`header`、`cookie`、`json`、`form`
   或 `file`。不要在 Controller 中再次手工解析已声明字段。
4. 使用 `binding` 表达绑定期必填约束，使用 `validate` 表达范围、长度和格式约束；零值有业务意义
   时使用指针或显式存在性模型。
5. 让 `Res` 表达公开响应契约，不直接暴露数据库 Entity、DAO 类型或敏感内部字段。

## 不变量

- 路由中的 `:id`、`*path` 必须和 `path:"id"`、`path:"path"` 字段一一对应。
- JSON 依赖标准 `json` 标签及请求 Content-Type；它不等同于 query/form 绑定。
- PATCH 或可选更新必须区分“未提供”和“提供零值”，通常使用指针字段配合 `omitempty` 校验。
- `operationId` 在文档中应唯一；`tags`、`summary`、`description` 应描述业务动作而非 Go 类型。
- 请求结构变化后必须重新运行 Controller 生成预览，并更新绑定、校验和 OpenAPI 测试。

如果本地有 `gx`，只用它同步 Controller 契约，不把生成器当作 API 设计器。先修改开发者拥有的
API 定义，再运行 `gx ctrl --dry-run` 和 `gx ctrl`。不要手改 `*_generated.go`。
