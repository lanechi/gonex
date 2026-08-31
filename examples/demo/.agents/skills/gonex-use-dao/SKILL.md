---
name: gonex-use-dao
description: 在 gonex 项目中使用 gx dao 生成和调用 GORM DAO、Entity；适用于数据库表同步、数据访问查询和生成结果维护。
---

# 使用 gonex DAO

## 生成流程

`gx dao` 从项目根目录的 `DATABASE_*` 环境变量和 `.env` 读取数据库连接，不读取 `config.yaml`。修改表结构或
需要重新生成时，先预览：

```bash
gx dao --dry-run
gx dao
```

指定表时：

```bash
gx dao --tables users,orders
gx dao --tables public.users,billing.invoices
```

生成结果位于 `internal/dao` 和 `internal/model/entity`，两个目录必须成对更新；生成文件带
`DO NOT EDIT`，不要手工修改。`gx dao` 失败时应保留现有生成结果，先修正连接、表名或 schema 后重试。

## 使用边界

- Logic 负责调用 DAO、组合查询、事务和领域规则；Controller 不直接访问 DAO 或 GORM。
- 先阅读实际生成的 DAO 方法和 Entity 类型，不凭记忆假定方法名；用稳定的 Repository/端口隔离可替换的数据访问。
- 不让 API Req/Res 或数据库 Entity 直接成为跨层稳定契约；在 Logic 中做输入和输出映射。
- 查询和写入都透传 `context.Context`，尊重取消和 deadline；批量、分页和排序使用明确上限。
- 不把密码、Token 或完整敏感记录写入日志和响应。

## 依赖与验证

数据库驱动由应用 module 自己持有。生成结束后运行 `gofmt`、目标 module 的测试和 `go vet`，并确认
DAO、Entity、Logic、Service 和 Controller 一起编译。
