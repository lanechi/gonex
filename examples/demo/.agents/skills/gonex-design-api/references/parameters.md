# gonex API 参数与契约规则

## 1. g.Meta

每个请求结构体必须匿名嵌入 `g.Meta`。`path` 和 `method` 必填：

```go
type UpdateUserReq struct {
	g.Meta `path:"/users/:id" method:"patch" tags:"Users" summary:"Update user" operationId:"updateUser"`
	ID     int64   `path:"id" binding:"required" description:"User ID"`
	Name   *string `json:"name,omitempty" validate:"omitempty,min=1,max=80" description:"Display name"`
}
```

支持的元数据：

| 标签 | 用途 |
| --- | --- |
| `path` | 以 `/` 开头的路由；支持整个路径段形式的 `:name` 和末尾 `*name` |
| `method` | 单个 `GET`、`HEAD`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS` 或 `TRACE` |
| `tags` | OpenAPI 分组；逗号或空白分隔 |
| `summary` / `description` | 动作摘要和说明 |
| `operationId` | OpenAPI 操作标识；项目内保持唯一 |
| `deprecated` | `true` 或 `false` |
| `security` | 安全方案，可使用 `BearerAuth:scope` 形式 |
| `consumes` / `produces` | 媒体类型列表 |

不要把 RouterGroup 前缀重复写进 `path`。先检查项目现有 Bind 方式和路由测试。

## 2. 字段来源

| 来源 | 标签 | 常见类型与注意事项 |
| --- | --- | --- |
| 路径 | `path:"id"` | 标量或可文本解码类型；必须与路由通配符一一对应 |
| 查询 | `query:"page"` | 标量、指针、slice/array；重复 query key 可绑定 slice |
| Header | `header:"Authorization"` | 标量或 slice；名称按 HTTP Header 语义处理 |
| Cookie | `cookie:"session"` | 通常为 string；敏感值不要进入日志或响应 |
| JSON | `json:"name"` | 由 JSON Content-Type 驱动；可嵌套 struct、slice、map |
| 表单 | `form:"name"` | `application/x-www-form-urlencoded` 或 multipart |
| 文件 | `file:"avatar"` | `*multipart.FileHeader` 或其 slice，仅 multipart |

path/query/header/cookie/form 支持 string、bool、整数、无符号整数、浮点、`time.Duration`、实现
`encoding.TextUnmarshaler` 的类型，以及这些类型的指针、slice/array。解析失败会成为 400 绑定错误。

同一字段不要声明多个竞争来源。Binder 按 path、query、header、cookie、form 的顺序使用第一个实际
存在的值，这会让契约难以理解；需要同时支持多个公开字段时应显式建立映射逻辑和测试。

## 3. 校验

- `binding` 用于绑定层规则和必填语义，例如 `required`。
- `validate` 用于业务形状，例如 `gte`、`lte`、`min`、`max`、`oneof`、格式校验。
- 两组规则都在 Controller 调用前执行；Controller 不应重复同一校验。
- 项目通过 `WithValidator` 注入同一 Validator 时，两组标签仍分别执行；自定义规则必须注册在该实例。
- 对 string/slice/map，`min`、`max` 表示长度；对数字表示数值边界。
- 当 `0`、`false` 或空字符串是合法的显式输入时，使用指针区分缺失值，避免 `required` 把合法零值
  判为空。
- PATCH 字段通常使用指针和 `omitempty`；PUT 是否允许省略字段由业务契约决定，不机械套用 PATCH。

路径参数天然必须存在，但仍建议加 `binding:"required"` 以表达文档和校验契约。

## 4. OpenAPI 字段信息

字段支持：

- `description` 或 `dc`：说明；优先使用 `description`；
- `example`：示例；
- `default`：默认值，仅描述契约，不自动给字段赋值；
- `enum`：枚举值；
- `binding` / `validate` 中的边界会投影为 schema 约束。

OpenAPI 文档不是替代测试的注释层。修改字段后检查 `/openapi.json` 中参数位置、required、schema、
媒体类型和安全方案是否与实际 Binder 一致。

## 5. 常用模型

### 查询与分页

```go
type ListUserReq struct {
	g.Meta   `path:"/users" method:"get" tags:"Users" summary:"List users" operationId:"listUsers"`
	Page     int      `query:"page" validate:"gte=1" default:"1"`
	PageSize int      `query:"pageSize" validate:"gte=1,lte=100" default:"20"`
	Status   []string `query:"status" validate:"omitempty,dive,oneof=active disabled"`
}
```

若应用需要真正的默认值，必须在 Controller/应用层显式归一化，`default` 只影响文档。

### JSON 创建

```go
type CreateUserReq struct {
	g.Meta `path:"/users" method:"post" tags:"Users" summary:"Create user" operationId:"createUser" consumes:"application/json"`
	Name   string `json:"name" binding:"required" validate:"min=1,max=80"`
	Email  string `json:"email" binding:"required" validate:"email"`
}
```

### Multipart 上传

```go
type UploadAvatarReq struct {
	g.Meta `path:"/users/:id/avatar" method:"post" tags:"Users" summary:"Upload avatar" consumes:"multipart/form-data"`
	ID     int64                 `path:"id" binding:"required"`
	File   *multipart.FileHeader `file:"avatar" binding:"required"`
}
```

文件大小、类型和内容安全校验不能只依赖扩展名；遵循项目的请求体限制和存储策略。

## 6. 响应模型

- 使用专用响应 DTO，避免直接返回数据库 Entity。
- 列表明确 `items`、分页和 `total` 的语义；空列表使用项目约定的空 slice/null 行为。
- 时间、金额、ID 和枚举序列化格式保持项目一致。
- 不在业务 `Res` 中重复 gonex 默认的 `code/message/data` 包络，除非项目替换了 ResponseEncoder。
- 下载、流、HTML 和重定向属于直接响应，不需要伪造普通 JSON `Res`。
