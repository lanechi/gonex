---
name: gonex-use-config
description: 在 gonex 应用中读取和组织 config.yaml、.env 与系统环境变量，并在数据库或 Server 初始化前完成配置校验。
---

# 使用 gonex 配置

## 读取来源与优先级

默认使用 `g.Cfg()`。它会加载项目根目录的 `.env` 和第一个存在的默认配置文件：
`config.yaml`、`config/config.yaml`、`manifest/config/config.yaml`。

优先级为：

```text
config.Set() > 进程环境变量 > .env > 配置文件 > 默认值
```

YAML 使用嵌套 key；环境变量使用大写下划线。比如 `server.maxBodyBytes` 对应
`SERVER_MAX_BODY_BYTES`。环境变量覆盖具体 key，不替换整个配置对象。

## 启动顺序

如果数据库、缓存或其它基础设施在 Server 之前初始化，应先显式初始化并检查配置：

```go
if err := config.Init(); err != nil {
	return err
}
cfg := g.Cfg()

db, err := initDatabase(cfg)
if err != nil {
	return err
}

server := ghttp.NewServer(ghttp.WithConfig(cfg))
```

只创建默认 Server 时，`ghttp.NewServer()` 会自动初始化默认配置；不要在多个 Server 中维护多份
全局配置。测试或确实需要隔离时，使用 `config.Load(path)` 并将实例通过 `WithConfig` 注入。

## 约束

- 使用 `GetString`、`GetInt`、`GetBool` 或 `Unmarshal` 读取配置；结构体字段使用明确的
  `mapstructure` 标签。
- 不把密码、Token、Cookie secret 提交到 `config.yaml` 或 `.env`；生产环境优先使用部署平台 Secret
  注入进程环境。
- `.env` 是明文文件，生产环境若不希望读取它，不要部署该文件；框架不会自动按生产模式禁用它。
- 数据库生成器 `gx dao` 只读取 `DATABASE_*` 的进程环境变量和 `.env`，不会读取 `config.yaml`。
