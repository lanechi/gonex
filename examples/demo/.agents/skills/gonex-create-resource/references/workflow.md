# gonex 资源端到端工作流

本文件用于创建完整资源。只修改单层时，优先使用对应的专项 skill。

## 1. 项目预检

先确认这些事实：

- 最近的 `go.mod` 及 module path；
- 项目适用的 `AGENTS.md`；
- API 根目录、版本目录和路由分组前缀；
- Controller、Logic、Service、model、DAO 与测试的现有布局；
- 应用入口是否 blank-import `internal/logic`；
- 工作区是否已有未提交改动。

`gx` 从当前目录向上发现最近的 `go.mod`，生成路径以该 module root 为准；在子目录运行前仍要确认
发现的是目标 module，尤其是嵌套 module 仓库。

查找本地生成器：

```bash
command -v gx
gx --help
```

若 `gx` 不存在，不要擅自安装或联网。可以告诉用户安装命令是：

```bash
go install github.com/lanechi/gonex/gx@latest
```

然后按项目现有模式手工完成。手工创建的文件不要伪造 `DO NOT EDIT` 生成标记。

## 2. 选择生成模式

### 标准资源骨架

适合新模块或常规 CRUD 起点：

```bash
gx ctrl user/v1/order --dry-run
gx ctrl user/v1/order

gx service user --dry-run
gx service user
```

命名 `ctrl` 会创建标准 API、Controller 契约和首次动作实现；命名 `service` 会创建标准 Logic、
Service 和注册聚合。API、Controller 动作实现和 Logic 创建后由开发者维护；Controller 契约、
Service 与聚合文件由 gx 维护。

骨架生成后：

1. 把 API 占位字段改为真实 path/query/body/response 契约；
2. 把 Logic 的占位模型和空实现改为真实领域输入输出；
3. 在 Controller 动作实现中完成 API 与领域模型映射；
4. 用扫描模式重新生成真实契约：

```bash
gx ctrl --dry-run
gx service --module user --dry-run
gx ctrl
gx service --module user
```

不要重复使用命名命令来同步业务签名；后续同步使用无名称的扫描模式或 `--module`。

### 自定义动作

非 CRUD、已有模块新增动作或需要精确设计时：

1. 手写开发者拥有的 `api/<module>/<module>.go` 或 `api/<module>/<version>/<action>.go`；
2. 运行 `gx ctrl --dry-run`，确认只更新契约并首次创建缺少的动作实现；
3. 手写或扩展 `internal/logic/<module>` 的公开业务方法；
4. 运行 `gx service --module <module> --dry-run`；
5. 应用生成计划，再实现 Controller 映射。

```bash
gx ctrl --dry-run
gx ctrl
gx service --module user --dry-run
gx service --module user
```

如果生成计划出现意外的 `UPDATE`、`DELETE` 或跨模块文件，先检查当前目录、最近 `go.mod`、
`--dir` 和模块名，不要直接执行。

## 3. API 契约

每个动作定义 `ActionReq` 和 `ActionRes`。请求嵌入 `g.Meta`，字段来源和校验遵循
`gonex-design-api` 的参数规则。路径通配符与 `path` 字段必须一一匹配。

API 层只暴露 HTTP 契约。不要让请求结构体直接复用数据库 Entity，也不要把内部错误、密码、
令牌或审计字段放入响应。

## 4. Logic 与 Service

Logic 的导出 receiver 方法会被 `gx service` 扫描进 Service 接口。保持以下边界：

- 公开业务能力使用导出方法；
- 仅供包内使用的辅助方法必须非导出；
- 第一个参数使用 `context.Context`；
- 不依赖 API Req/Res 或 `ghttp.Context`；
- 通过 `service.Register<Module>(New())` 注册实现。

修改 Logic 方法签名后，总是重新生成对应 Service，并修复 Controller 调用处。

## 5. Controller 与路由注册

Controller 实现只承担 HTTP 边界。确认新 Controller 已传给 `Server.Bind`、`RouterGroup.Bind` 或
项目现有注册聚合，不要只生成文件而忘记启动绑定。

若项目使用路由前缀，`g.Meta.path` 通常表达组内路径；以现有相邻模块和实际路由测试为准，不猜测
最终 URL。

## 6. 所有权表

| 文件 | 默认所有者 | 修改方式 |
| --- | --- | --- |
| `api/<module>/<module>.go` | gx | 由 API 扫描结果维护接口；不要手改生成接口 |
| `api/<module>/<version>/*.go` | 开发者 | 直接编辑；标准骨架仅首次创建 |
| Controller `<module>.go`、`<module>_new.go` | gx | 修改 API 后运行 `gx ctrl` |
| Controller 动作实现 | 开发者 | 在首次生成文件中实现业务映射 |
| `internal/logic/<module>/*.go` | 开发者 | 直接编辑公开业务方法 |
| `internal/service/*.go` | gx | 修改 Logic 后运行 `gx service` |
| `internal/logic/logic.go` | gx | 由 `gx service` 同步 blank import |
| DAO / Entity 生成目录 | gx / GORM Gen | 通过 `gx dao` 重建 |

文件头的 `Code generated ... DO NOT EDIT.` 优先于表中的目录推断。

`gx dao` 会在 staging 中生成、合法化 struct tag 并校验，失败时回滚旧目录和 module 文件；成功时
仍会完整替换 DAO/Entity，不能把失败原子性理解为保留手写文件。

## 7. 验证清单

按变更风险选择验证，至少完成目标 module 的 `go test ./...`：

- 对修改的 Go 文件执行 `gofmt`；
- `go test ./...`，必要时 `go test -race ./...`、`go vet ./...`；
- HTTP 成功、缺失参数、非法参数、未找到、冲突和权限失败；
- path 参数与字段绑定、JSON Content-Type、分页边界；
- Service 注册和应用入口 blank import；
- `/openapi.json` 中的方法、路径、required、schema 和安全声明；
- 生成器再次 dry-run 时结果为预期或 unchanged；
- README/AGENTS 与实际命令、路由、目录同步。

修改 gonex 框架仓库时，还要分别测试核心 module、`gx` module 和受影响的独立 example module。
