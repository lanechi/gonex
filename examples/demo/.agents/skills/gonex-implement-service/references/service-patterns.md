# gonex Logic 与 Service 模式

## 1. 分层职责

Logic 是业务实现，Service 是生成的调用契约：

```text
internal/controller → internal/service → internal/logic → DAO / external ports
```

Logic 可以依赖 `internal/model`、生成 DAO 和外部端口，但不能依赖 `api/<module>/<version>` 的
Req/Res，也不能要求 `ghttp.Context`。HTTP 状态、Cookie、Header 和响应包络不属于 Logic。

项目基础设施使用唯一的全局入口：配置统一读取 `g.Cfg()`；数据库优先使用启动时初始化的 `dao.Q`，
生成 DAO 无法表达 Raw SQL、特殊锁或驱动能力时，允许使用项目现有的全局 DB accessor（demo 中为
`db.Postgres()`）。不要保存 `config.Config`、`*gorm.DB`、`dao.Query`，也不要创建包级副本。

除 bootstrap 初始化/关闭连接和 gx 生成 DAO 外，所有业务数据库读写必须位于 Logic。API、Controller、
Service、model、Middleware、定时任务入口不得直接查询数据库；它们必须调用 Service/Logic。

## 2. Logic 结构

每个 `internal/logic/<name>/` 目录只定义一个主要 Logic receiver，并对应一个生成 Service。典型目录：

```text
internal/logic/user/       # 一个 user Logic receiver
internal/logic/order/      # 一个 order Logic receiver
internal/logic/refund/     # 一个 refund Logic receiver
```

不要把 `sOrder`、`sRefund` 等多个服务 receiver 放进同一个 Logic package。一个 receiver 可以在本目录
拆成多个 `.go` 文件，但同目录不能再声明第二个服务结构体。典型实现：

```go
package user

type logic struct{}

func New() service.IUser {
	return &logic{}
}

func (*logic) Create(
	ctx context.Context,
	input *model.CreateUserInput,
) (*model.User, error) {
	query := dao.Q.User.WithContext(ctx)
	entity, err := query.Where(dao.Q.User.Email.Eq(input.Email)).First()
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &model.User{ID: entity.ID, Name: entity.Name}, nil
}

func (*logic) normalizeName(name string) string {
	return strings.TrimSpace(name)
}
```

示例中的 Query/字段名必须以实际 `gx dao` 输出为准。配置值直接从唯一入口读取，例如
`g.Cfg().GetInt("user.maxPageSize")`；高频调用可在方法内读取所需标量，但不把配置实例缓存到 Logic。

`gx service` 会扫描 Logic 包中带 receiver 的导出方法。`Create` 会进入 Service；`normalizeName`
不会。不要导出仅供内部使用的辅助方法，否则会无意扩大 Service 接口。

第一个参数应为 `context.Context`，并继续传给数据库和外部 I/O。不要保存 Context 到结构体，也不要
用 `context.Background()` 丢弃请求取消和超时。

## 3. 注册

每个 Logic 目录的注册方式：

```go
func init() {
	service.RegisterUser(New())
}
```

如果需要 Order 和 Refund 两个服务，必须分别放在 `internal/logic/order` 与
`internal/logic/refund`，各自注册：

```go
// internal/logic/order
func init() { service.RegisterOrder(New()) }

// internal/logic/refund
func init() { service.RegisterRefund(New()) }
```

应用入口还需 blank-import Logic 聚合包：

```go
import _ "example.com/app/internal/logic"
```

`gx service` 会维护 `internal/logic/logic.go` 的模块 blank import。DAO、全局 DB 和配置不通过 `New` 注入；启动
入口必须在接收请求前完成 `config.Init()`、数据库初始化和 `dao.SetDefault(database)`。需要外部客户端
时复用项目已有的全局基础设施入口，不在每个 Logic 构造函数中创建连接或维护第二份配置。

删除整个 Logic 模块后再次运行 `gx service`，聚合器会移除该模块的 gx 受管 blank import，同时保留
其它用户 import 和代码；不要手工和生成器争用这些受管条目。

出现 `gx: <module> service is not registered` 时按顺序检查：

1. Logic 包是否被聚合文件 blank-import；
2. 应用入口是否导入聚合包；
3. 对应 Logic 目录的 `init` 是否调用了正确的 `Register<Name>`；
4. 是否存在循环依赖导致采用了错误包；
5. 测试是否绕过正常启动入口且没有注册 fake。

## 4. 生成 Service

修改 Logic 导出方法后：

```bash
gx service --module user --dry-run
gx service --module user
```

检查计划是否只更新目标 `internal/service/user.go` 和必要的聚合 import。Service 文件带
`DO NOT EDIT`，必须通过 Logic 签名重生成。

命名模式 `gx service user` 只适合首次创建标准骨架。业务化后使用 `--module user`，避免重新套用
占位 CRUD 签名。

生成失败的常见原因：

- 同一目录误放多个 receiver，或不同文件重复声明导出方法；
- Logic 方法签名引用了无法解析或包名冲突的类型；
- 目标模块目录与 Go package 名不一致；
- 当前目录向上发现了错误的 `go.mod`；
- Logic 目录不存在或没有可扫描模块。

## 5. 全局 DAO、配置、模型与事务

- 启动时只初始化一次默认配置、数据库连接和 `dao.Q`；Logic 只消费这些全局入口，不负责初始化或关闭。
- 查询优先从 `dao.Q.<table>.WithContext(ctx)` 开始；生成 DAO 缺少所需能力时，才从全局
  `db.Postgres().WithContext(ctx)` 开始 GORM/Raw SQL 操作。
- 事务位于 Logic，优先使用生成 DAO 的全局 Query；确需全局 DB transaction 时也必须限定在 Logic
  方法内。transaction handle 不得逃逸，Controller 不参与事务。
- 任何 Logic 都不得调用 `gorm.Open`、`sql.Open`，不得定义新的 DAO/Repository 或缓存数据库连接。
- 为用例定义输入/输出模型。跨层复用的导出结构体放在 `internal/model`，按领域拆文件；不要在 Logic、
  Controller 或 Service 中重复声明同形结构体。
- API 专用 Req/Res 留在 `api`，数据库 Entity 留在 `internal/model/entity`；二者都不能替代稳定业务模型。
- 跨多个写操作的原子性在 Logic 编排，事务生命周期不能逃逸到 HTTP 层。
- 使用可通过 `errors.Is` / `errors.As` 识别的领域错误，让 Controller 安全映射。
- 幂等、资源归属和状态转换属于 Logic；字段格式等机械结构校验可以留在 API 绑定层。
- 不记录密码、Token、Cookie、完整支付数据或不必要的请求体。

## 6. 测试

Logic 测试不启动 HTTP Server。为 `dao.Q` 配置隔离的测试数据库，并设置/恢复测试所需全局配置；
修改全局入口的测试不要并行。外部端口可使用 fake，覆盖：

- 正常业务路径和状态变化；
- 领域冲突、未找到、权限和幂等；
- DAO/外部依赖错误的保留与分类；
- Context 取消或 deadline；
- 事务提交/回滚边界（若适用）。

生成后再运行整个应用 module 测试，确保 Service 接口、Logic 注册和 Controller 调用一起编译。
