# gonex Logic 与 Service 模式

## 1. 分层职责

Logic 是业务实现，Service 是生成的调用契约：

```text
internal/controller → internal/service → internal/logic → DAO / external ports
```

Logic 可以依赖领域模型、Repository/DAO 抽象和外部端口，但不能依赖 `api/<module>/<version>` 的
Req/Res，也不能要求 `ghttp.Context`。HTTP 状态、Cookie、Header 和响应包络不属于 Logic。

## 2. Logic 结构

遵循项目现有构造方式。典型模块：

```go
package user

type sUser struct{}

func New() service.IUser {
	return &sUser{}
}

func (logic *sUser) Create(
	ctx context.Context,
	input *model.CreateUserInput,
) (*model.User, error) {
	// Apply business rules and call the project's existing repository/DAO boundary.
	return nil, nil
}

func (logic *sUser) normalizeName(name string) string {
	return strings.TrimSpace(name)
}
```

`gx service` 会扫描 Logic 包中带 receiver 的导出方法。`Create` 会进入 Service；`normalizeName`
不会。不要导出仅供内部使用的辅助方法，否则会无意扩大 Service 接口。

第一个参数应为 `context.Context`，并继续传给数据库和外部 I/O。不要保存 Context 到结构体，也不要
用 `context.Background()` 丢弃请求取消和超时。

## 3. 注册

无参数构造的常见注册方式：

```go
func init() {
	service.RegisterUser(New())
}
```

应用入口还需 blank-import Logic 聚合包：

```go
import _ "example.com/app/internal/logic"
```

`gx service` 会维护 `internal/logic/logic.go` 的模块 blank import。带外部依赖的项目可使用项目现有的
组合根显式构造并调用 `service.RegisterUser(user.New(repository))`；此时不要同时保留会注册另一实现的
`init`。不要为了套用 `init` 隐藏数据库、网络 client 或可失败初始化。

删除整个 Logic 模块后再次运行 `gx service`，聚合器会移除该模块的 gx 受管 blank import，同时保留
其它用户 import 和代码；不要手工和生成器争用这些受管条目。

出现 `gx: <module> service is not registered` 时按顺序检查：

1. Logic 包是否被聚合文件 blank-import；
2. 应用入口是否导入聚合包；
3. `init` 或显式组合根是否调用正确的 `Register<Module>`；
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

- 同一模块存在重名导出 receiver 方法；
- Logic 方法签名引用了无法解析或包名冲突的类型；
- 目标模块目录与 Go package 名不一致；
- 当前目录向上发现了错误的 `go.mod`；
- Logic 目录不存在或没有可扫描模块。

## 5. 模型、事务和错误

- 为用例定义输入/输出模型，不把 API Req/Res 或数据库 Entity 直接当稳定 Service 契约。
- Repository/DAO 的具体选择遵循项目现有模式；Controller 不参与事务。
- 跨多个写操作的原子性在 Logic 编排，事务生命周期不能逃逸到 HTTP 层。
- 使用可通过 `errors.Is` / `errors.As` 识别的领域错误，让 Controller 安全映射。
- 幂等、资源归属和状态转换属于 Logic；字段格式等机械结构校验可以留在 API 绑定层。
- 不记录密码、Token、Cookie、完整支付数据或不必要的请求体。

## 6. 测试

Logic 测试不启动 HTTP Server，使用 fake Repository/端口覆盖：

- 正常业务路径和状态变化；
- 领域冲突、未找到、权限和幂等；
- Repository/外部依赖错误的保留与分类；
- Context 取消或 deadline；
- 事务提交/回滚边界（若适用）。

生成后再运行整个应用 module 测试，确保 Service 接口、Logic 注册和 Controller 调用一起编译。
