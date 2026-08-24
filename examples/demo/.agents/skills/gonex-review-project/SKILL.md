---
name: gonex-review-project
description: 审查 gonex 项目的 API、Controller、Logic、Service、生成文件和同步完整性；适用于代码评审、一致性检查和框架规范审计，不用于普通实现任务。
---

# 审查 gonex 项目

审查前阅读 [references/conventions.md](references/conventions.md)。默认只报告问题；只有用户明确要求修复
时才改代码。

## 审查方法

1. 读取全部适用的 `AGENTS.md`、`go.mod` 和变更差异，区分用户已有改动与本次变更。
2. 沿每个受影响动作追踪：`Req/Res → Controller → Service → Logic → 注册 → 测试/文档`。
3. 先检查生成文件所有权，再评估实现；任何手改 `DO NOT EDIT` 文件都视为同步问题。
4. 对框架仓库变更额外检查核心测试、`gx` 模板/生成结果、相关 `examples`、README 和架构文档。

## 优先级

- 严重：安全绕过、数据损坏、生成器破坏手写文件、路由或 Service 在运行时不可用。
- 高：API 参数绑定错误、路径参数不匹配、Controller 绕过 Service、Logic 未注册、错误响应泄漏。
- 中：生成契约过期、Context 未透传、校验/OpenAPI/测试与行为不一致、同步文件遗漏。
- 低：不影响契约的局部可维护性问题。

每条发现必须给出文件与行号、可观察影响、触发条件和最小修复方向。不要把风格偏好当缺陷；没有
发现时明确说明剩余风险和未运行的验证。

## 重点不变量

- 请求字段来源和 `g.Meta` 与实际 HTTP 路由一致，path 参数一一对应。
- Controller 不访问 DAO，Logic 不依赖 API 或 HTTP Context。
- Service 接口来自 Logic 导出签名，注册链完整。
- 直接写响应后不会再返回触发统一编码的值或错误。
- 生成命令先 dry-run；`--clean` 和 `dao` 的破坏范围经过确认。
- 行为变更有测试、README；框架变更还同步 `gx` 和至少一个可运行 example。
