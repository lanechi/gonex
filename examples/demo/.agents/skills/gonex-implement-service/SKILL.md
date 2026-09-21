---
name: gonex-implement-service
description: 实现 gonex 业务 Logic、数据库操作、生成 Service 接口并完成注册；适用于新增业务方法、在 Logic 中使用 DAO/全局 DB、调整领域模型或排查 Service 未注册问题。
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
- `internal/logic/<name>/` 只放一个 Logic 类（主要 receiver）；一个 Logic 结构体对应一个目录。需要
  第二个 receiver 时创建新的 Logic 目录和独立 Service，不在同一个 package 中堆叠多个服务类。
- 跨 Controller、Service、Logic 或多个 Logic 目录复用的导出结构体统一定义在 `internal/model`；
  不在 Logic 目录声明第二个业务结构体。
- Service 文件是 Logic 导出方法签名的生成投影；业务实现不写进 Service 生成文件。
- 每个 Logic 目录通过 `service.Register<Name>(New())` 注册唯一实现。应用启动入口始终 blank-import
  `internal/logic` 聚合包。
- 所有业务数据库查询、写入、事务和 Raw SQL 都写在 Logic。优先使用启动时初始化的 `dao.Q`；只有
  生成 DAO 无法表达操作时才使用项目已有的全局 DB。不要调用 `gorm.Open`、`sql.Open`，不要创建
  DAO/Repository 或连接副本。
- 配置读取统一使用 `g.Cfg()`；不要在 Logic 结构体中保存配置或数据库状态，也不要另建全局变量。
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
`service is not registered` 时，依次检查 Logic 的 `init`、对应 receiver 的注册函数、聚合 blank import
和启动入口。`gx service` 会保留方法签名中的显式 import alias；每个 Logic 目录的唯一 receiver 生成
对应的 `I<Name>`。
