# basic

`basic` 是 gonex 的最小可运行示例，展示从请求声明到 HTTP 响应的完整链路。

## 展示内容

- 使用 `ghttp.NewServer` 创建独立 Server；
- 使用 `g.Meta` 声明 method、path 和 OpenAPI 元数据；
- 使用 `RouterGroup.Bind` 注册 Controller；
- 将 query 参数绑定到请求结构体；
- 验证 RouterGroup 前缀中的 path 参数可以绑定到请求结构体；
- 验证可选 query 参数缺失时使用 `default`；
- 返回统一响应并自动生成 OpenAPI/Swagger。

本目录是独立 module `github.com/lanechi/gonex/examples/basic`，通过 `go.mod` 中的本地
`replace` 使用仓库内 gonex。

## 运行

从 gonex 仓库根目录执行：

```bash
cd examples/basic
go run .
```

默认地址：

```text
GET http://localhost:8000/hello
GET http://localhost:8000/hello?name=Lane
GET http://localhost:8000/openapi.json
GET http://localhost:8000/docs/
```

`/hello?name=Lane` 的默认响应：

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "message": "Hello, Lane!"
  }
}
```

## 代码入口

[`main.go`](main.go) 同时定义 `HelloReq`、`HelloRes`、Controller 和 Server 启动代码，适合用来
验证公共 API 的最小改动；`main_test.go` 还覆盖了带 path 参数的 RouterGroup 前缀。

## 验证

```bash
go test ./...
go vet ./...
```

路由、绑定、默认响应或 OpenAPI 的公共契约变化时，必须同步本示例和 README；如果新行为需要
配置、数据库或多 Server，请改用 `quick-demo` 承载证明。
