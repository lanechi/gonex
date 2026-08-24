# gonex performance benchmark

本目录是独立 benchmark module，用于对比本地 gonex 与直接使用 Gin。

`go.mod` 通过 `replace github.com/lanechi/gonex => ../..` 使用当前工作区代码。

## 运行

从 gonex 仓库根目录执行：

```bash
cd benchmarks/gx
GIN_MODE=release go test . \
  -run '^$' \
  -bench 'Benchmark(Framework|Gin)' \
  -benchmem \
  -benchtime=3s
```

如果只需确认 benchmark 可以编译：

```bash
go test . -run '^$'
```

## 场景

- Gin 直接 handler；
- gonex Controller 与统一响应；
- 包含 JSON 绑定的 typed 请求场景。

每个 benchmark 在计时前都会探测响应正确性。简单请求为不同框架复制等价 request；JSON 绑定
场景每次迭代创建新 request，避免复用请求对象残留状态造成偏差。

## 维护规则

- 只比较语义等价的路径、绑定和响应；
- 不提交缺少 Go 版本、CPU、操作系统和 benchmark 参数的孤立数字；
- 热路径、反射、路由注册或响应编码变化时，先运行正确性测试，再运行 benchmark；
- benchmark 场景变化时同步本文和 `benchmark_test.go`。
