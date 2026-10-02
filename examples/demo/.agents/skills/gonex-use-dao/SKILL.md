---
name: gonex-use-dao
description: 在 gonex 项目中用 gx dao 生成 GORM DAO/Entity，并在 Logic 中通过 dao.Q 或全局 DB 查询；适用于表结构同步和业务数据库操作，不用于创建连接。
---

# 使用 gonex DAO

## 生成流程

`gx dao` 从项目根目录的 `DATABASE_*` 环境变量和 `.env` 读取数据库连接，不读取 `config.yaml`。
该命令没有 `--dry-run`，修改表结构或重新生成前必须先确认数据库目标、schema、表范围和工作区：

```bash
gx dao
```

指定表时：

```bash
gx dao --tables users,orders
gx dao --tables public.users,billing.invoices
```

生成结果位于 `internal/dao` 和 `internal/model/entity`，两个目录必须成对更新；生成文件带
`DO NOT EDIT`，不要手工修改。`gx dao` 失败时应保留现有生成结果，先修正连接、表名或 schema 后重试。

## 全局初始化

应用只初始化一份数据库和生成 DAO。启动顺序固定为：

```go
if err := config.Init(); err != nil {
	return err
}
configuration := g.Cfg()
if err := db.InitializePostgres(configuration); err != nil {
	return err
}
dao.SetDefault(db.Postgres())
```

必须在 Server 接收请求前完成 `dao.SetDefault`，并由启动层在停止时关闭数据库。不要在 Logic、
Controller 或请求处理中再次调用 `dao.SetDefault`、`dao.Use`、`gorm.Open`、`sql.Open` 或打开连接。

单 schema 的平铺输出使用 `internal/dao` 的 `Q`。多 PostgreSQL schema 会生成多个 DAO 子包，启动时
用同一数据库分别调用各包的 `SetDefault`，Logic 通过清晰 import alias 使用对应包的全局 `Q`；不要
把它们再汇总成另一层 Repository singleton。

## 使用边界

- 除 bootstrap 初始化连接和 gx 生成 DAO 外，所有业务查询、写入、事务、Raw SQL 必须写在 Logic；
  API、Controller、Service、model、Middleware 和任务入口不得直接操作数据库。
- Logic 优先通过全局 `dao.Q` 调用 DAO。生成 DAO 无法表达所需操作时，允许使用项目已有的全局 DB
  accessor（demo 为 `db.Postgres()`）；禁止自行创建 DAO、Repository、DB wrapper 或连接。
- 每次查询从 `dao.Q.<table>.WithContext(ctx)` 开始。先阅读实际生成的 DAO 方法和 Entity 类型，不凭
  记忆假定 Query、字段或方法名；使用全局 DB 时同样必须先调用 `WithContext(ctx)`。
- 不让 API Req/Res 或数据库 Entity 直接成为跨层稳定契约；跨层复用的结构体统一放
  `internal/model`，由 Logic 完成 Entity 与公共模型映射。
- 查询和写入都透传 `context.Context`，尊重取消和 deadline；批量、分页和排序使用明确上限。
- 不把密码、Token 或完整敏感记录写入日志和响应。

全局 DB fallback 只能出现在 Logic，且必须有 DAO 无法满足的明确理由：

```go
func (*logic) Summary(ctx context.Context, id int64) (*model.UserSummary, error) {
	var result model.UserSummary
	database := db.Postgres().WithContext(ctx)
	if err := database.Raw(summarySQL, id).Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("query user summary: %w", err)
	}
	return &result, nil
}
```

若 `dao.Q` 已能完成同一操作，必须使用 DAO，不要默认编写 Raw SQL。

## PostgreSQL 数组

PostgreSQL 标准数组由 `gx dao` 读取 `pg_catalog.pg_type` 的 OID、`typcategory`、`typelem`
和元素类型后生成；不要根据 `_text`、`text[]` 等字符串自行猜测类型，也不要手工修改生成的
Entity。

生成约定：

```text
text[]             -> pgtype.Array[string]
bigint[]           -> pgtype.Array[int64]
boolean[]          -> pgtype.Array[bool]
double precision[] -> pgtype.Array[float64]
```

不要把这些字段改成裸 `[]T`、`pgtype.FlatArray[T]` 或 `pq.*Array`。当前 GORM 会在写 SQL
时展开 slice；`pgtype.Array[T]` 是 pgx 自带结构类型，可以作为单个参数交给 pgx ArrayCodec。
`Valid=false` 表示 SQL NULL；一维非 NULL 数组要同时提供 `Elements`、`Dims` 和 `Valid=true`。
查询、Create、Save 和 Updates 都直接使用生成 Entity 中的 `pgtype.Array[T]` 字段，不再增加
Repository 或自定义 Scanner/Valuer 适配层。

## 依赖与验证

数据库驱动和连接生命周期由应用启动层持有。生成结束后运行 `gofmt`、目标 module 的测试和
`go vet`，并确认 DAO、Entity、全局初始化、Logic、Service 和 Controller 一起编译。
