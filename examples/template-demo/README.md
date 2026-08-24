# template-demo

`template-demo` 展示 gonex 对 Go 标准库 `html/template` 的封装和模板文件热加载。

## 展示内容

- 通过 `config/config.yaml` 设置 `server.template.root` 和 reload；
- 通过 `server.AddTemplateFunc` 注册经过校验的 `upper` 模板函数；非法名称、nil 或非法签名会返回错误；
- Controller 使用 `ghttp.FromContext(ctx).HTML` 渲染模板；
- `fsnotify` 监听模板文件变化并清理缓存；
- 下一次请求重新解析模板，无需重启进程。

本目录是独立 module `github.com/lanechi/gonex/examples/template-demo`，通过本地 `replace` 使用
仓库内 gonex。

## 运行

从 gonex 仓库根目录执行：

```bash
cd examples/template-demo
go run .
```

访问：

```text
http://localhost:8002/page?name=Lane
```

启动后修改 `resource/template/index.html` 并刷新页面，即可看到新内容。

## 关键文件

```text
config/config.yaml
internal/controller/page.go
resource/template/index.html
main.go
main_test.go
```

## 验证

```bash
go test ./...
go vet ./...
```

模板根目录、reload、Context 渲染 API 或错误语义变化时，必须同步本示例、`main_test.go` 和 README。
