---
name: gonex-use-template
description: 在 gonex Controller 中配置、渲染和扩展 HTML 模板；适用于页面响应、模板函数和模板热更新。
---

# 使用 gonex 模板

## 配置模板根目录

模板根目录可以通过 `config.yaml` 配置：

```yaml
server:
  template:
    root: resource/template
```

也可以在创建 Server 时显式指定：

```go
server := ghttp.NewServer(ghttp.WithTemplateRoot("resource/template"))
```

配置或选项指定的目录必须存在且至少包含一个 `.html` 文件。模板按相对模板根目录的路径命名。

## Controller 渲染

页面 Controller 使用 `ghttp.FromContext(ctx)` 获取框架上下文，再调用 `HTML`：

```go
requestContext := ghttp.FromContext(ctx)
if requestContext == nil {
	return nil, errors.New("gonex context is unavailable")
}
if err := requestContext.HTML(http.StatusOK, "index.html", data); err != nil {
	return nil, err
}
return nil, nil
```

直接写出 HTML 后不要再返回响应对象，避免重复响应。

## 模板函数与更新

在模板根目录加载前或修改期间注册函数：

```go
if err := server.AddTemplateFunc("upper", strings.ToUpper); err != nil {
	return err
}
```

模板 Manager 会维护解析快照并监视目录变化；模板文件修改后会失效缓存，下一次执行时重新解析。
函数必须可被 `html/template` 接受，且不能依赖未同步保护的可变共享状态。

不要在 Controller 中直接读取模板文件或绕过 `server.Templates()` 管理缓存；需要高级集成时使用
Server 暴露的 Manager API。
