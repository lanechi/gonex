---
name: gonex-implement-service
description: 实现 gonex 业务 Logic、生成 Service 接口并完成注册；适用于新增业务方法、调整领域模型或排查 Service 未注册问题。
---

# 实现 gonex Logic 与 Service

开始前阅读 [references/service-patterns.md](references/service-patterns.md)，并检查现有
`internal/logic/<module>`、`internal/service`、模型与数据访问边界。

## 依赖方向

```text
Controller → generated Service interface → developer Logic → DAO / external ports
```

- 在 Logic 上实现公开方法，并把 `context.Context` 放在第一参数。
- Logic 使用领域/应用模型，不依赖 `api/...` 的 Req/Res，不接收 `ghttp.Context`。
- Service 文件是 Logic 导出方法签名的生成投影；业务实现不写进 Service 生成文件。
- 每个 Logic 模块通过 `service.Register<Module>(New())` 注册；应用启动入口 blank-import
  `internal/logic` 聚合包。
- 事务、幂等、授权后的业务规则和跨 DAO 编排属于 Logic；HTTP 状态和响应结构属于 Controller。

## gx 工作流

若本地存在 `gx`，先运行 `gx service --help`。修改开发者拥有的 Logic 后执行：

```bash
gx service --module <module> --dry-run
gx service --module <module>
```

使用命名命令创建标准骨架时，也必须先 dry-run；骨架创建后在开发者拥有的 Logic 中替换占位模型和
实现，再用 `--module` 从真实签名重生成 Service。不要手改带 `DO NOT EDIT` 的 Service 或 Logic
聚合文件。`gx` 不存在时按相邻模块手工创建 Logic 和注册代码，不自动安装。

## 验证

至少覆盖业务成功、领域失败、Context 取消和关键数据边界。运行格式化与目标 module 测试；出现
`service is not registered` 时，依次检查 Logic 的 `init`、注册函数、聚合 blank import 和启动入口。
