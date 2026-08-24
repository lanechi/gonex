# gonex examples

`examples` 提供四个独立 Go module；其中 `demo` 是唯一由 `gx init` 使用的项目模板，其余目录是
可运行示例。它们既是入门示例，也是核心框架与 `gx` 生成器的跨模块契约验证，同时是
[`examples/demo/.agents/skills`](demo/.agents/skills/) 工作流引用的可运行模式。

```text
examples/
├── demo/
├── basic/
├── quick-demo/
└── template-demo/
```

仓库没有 `go.work`。每个 example 的 `go.mod` 都通过
`replace github.com/lanechi/gonex => ../..` 使用本地框架，必须进入各自目录运行 Go 命令。

## 示例选择

| 示例 | 默认地址 | 证明的能力 |
| --- | --- | --- |
| [`demo`](demo/README.md) | `:8000` | `gx init` 唯一规范模板、API→Controller→Service→Logic、PostgreSQL |
| [`basic`](basic/README.md) | `:8000` | 最小 Server、`g.Meta`、query 绑定、统一响应、OpenAPI |
| [`quick-demo`](quick-demo/README.md) | `:8000`/`:8001`/`:8002` | 分层 API→Service→Logic、PostgreSQL、GORM、多 Server 和持续生成 |
| [`template-demo`](template-demo/README.md) | `:8002` | HTML 模板、配置、渲染和模板热加载 |

## 运行

最小示例：

```bash
cd examples/basic
go run .
```

完整分层示例：

```bash
cd examples/quick-demo
go run . serve
```

模板示例：

```bash
cd examples/template-demo
go run .
```

## 使用本地 gx

从 gonex 仓库根目录构建：

```bash
cd gx
go build -o /tmp/gx .
cd ../examples/quick-demo
```

预览并同步生成结果：

```bash
/tmp/gx ctrl --dry-run
/tmp/gx service --dry-run
/tmp/gx ctrl
/tmp/gx service
```

数据库模型生成依赖 quick-demo 根目录 `.env` 或系统 `DATABASE_*` 配置：

```bash
/tmp/gx dao --tables users
```

`gx dao` 在 staging 校验成功后成对替换 `internal/dao` 与 `internal/model/entity`，失败会回滚；
成功执行仍会完整重建。执行前阅读 [`gx/README.md`](../gx/README.md) 的所有权规则。

## 全量验证

```bash
cd examples/basic
go test ./...

cd ../quick-demo
go test ./...

cd ../template-demo
go test ./...
```

## 同步规则

框架公共行为变化时，必须更新至少一个能覆盖该行为的 example：

- 路由、绑定、响应、Middleware：优先更新 `basic` 或 `quick-demo`；
- 配置、数据库脚手架、生成目录：更新 `quick-demo`，并与 `gx init` 模板对齐；
- 模板能力：更新 `template-demo`；
- 所有 example 的命令、端口、路由或目录变化：同步本 README 和该 example README。
- API/Controller/Logic/Service 的推荐写法变化：同步对应 example 和 `demo/.agents/skills/` 中的相关 skill，确保 AI 规则
  可以由实际代码验证。

纯内部重构若无需修改 example，也必须重新运行相关 example 测试并在交付说明中记录原因。
