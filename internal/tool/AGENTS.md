# internal/tool 开发约定

日期：2026-06-03

## 目标与范围

本文档只记录 `internal/tool/` 这一层当前应长期遵守的开发约定。

它的目标是帮助后续协作者在新增或修改本地 tool 时，先统一边界和实现面，再进入具体编码。本文档偏向稳定约定，而不是当前实现说明。

本文档重点回答以下问题：

- 新增一个本地 tool 时需要实现哪些接口
- 一个本地 tool 的职责边界应该落在哪里
- `Metadata()` 应该如何书写
- `Execute()` 的错误分类、结果组织和最小测试要求是什么

本文档不承担以下职责：

- 不复述仓库通用分层规范；项目级通用约定仍以 [AGENTS.md](/root/agentGo/AGENTS.md) 为准
- 不覆盖 MCP remote tool 的接入细节
- 不规定 prompt 如何消费 tool metadata
- 不把当前各个具体 tool 写成逐文件说明书

## Tool 层总体边界

- `internal/tool/` 负责向 agent 暴露可调用工具，并封装单个工具的参数解码、业务校验、执行和结果组织
- tool 层可以依赖必要的底层能力或存储接口，但不应反向承接上层业务编排职责
- 单个 tool 应聚焦一个清晰动作，不要把多个独立能力堆进同一个工具
- 本地 tool 的实现事实源在 `internal/tool/` 内；跨模块共享的调用契约放在 `internal/tool/contract`

## 新增本地 Tool 的最小实现面

- 每个本地 tool 都应实现 [tool.go](/root/agentGo/internal/tool/tool.go) 中的 `Tool` 接口：
  - `Metadata() toolcontract.Metadata`
  - `Execute(context.Context, toolcontract.ToolCallRequest) toolcontract.ToolResult`
- 新 tool 默认放在 `internal/tool/` 根目录下，文件命名应与工具语义一致
- 如某个 tool 需要局部 helper、参数结构或小型私有类型，应优先保留在同文件或同目录内，不要过早抽成跨层公共包
- 每个 tool 应有稳定的工具名常量，避免在多处硬编码

## Tool 的职责分工

- tool 自己负责：
  - 参数 JSON 解码
  - 业务级输入校验
  - 路径、范围、资源等运行边界校验
  - 执行核心动作
  - 组装 `ToolResult`
- `tool/service` 负责：
  - tool 注册与列举
  - 基于 `Metadata` 的权限过滤
  - 基于 `Metadata.Parameters` 的基础 schema 校验
  - 基于 `IsConcurrencySafe` 的并发调度
- `lifecycle` 负责：
  - 按配置决定某个 tool 是否注册
  - 提供启动期 wiring，不承接 tool 内部业务逻辑

## `Metadata()` 书写规范

`Metadata()` 是一个本地 tool 对外暴露的最小契约。它必须自解释，不能把关键边界只藏在 `Execute()` 的运行时逻辑里。

### `Name`

- 名称必须稳定、简短、直接
- 默认使用当前本地 tool 的小写命名风格
- 名称应表达动作或能力本身，不要携带临时实现细节

### `Description`

- 必须先写清“这个 tool 用来做什么”
- 必须写清使用边界，而不是只写底层技术动作
- 对有明显副作用、权限前提、路径范围、网络依赖的 tool，应在描述里直接说明
- 描述应帮助调用方判断“什么时候该用它”，而不是罗列实现步骤
- 不写含糊短句，不堆砌内部实现术语

### `Parameters`

- 必须是合法的 object JSON Schema
- 每个暴露给调用方填写的字段都必须写 `description`
- 每个字段的 `description` 至少应说明：
  - 该字段的含义
  - 取值语义或边界
  - 对行为的主要影响
- `required` 必须显式列出；不要依赖调用方猜测必填项
- 对有限取值、路径限制、格式要求、默认行为，应尽量在字段描述中直接表达
- 若某个字段容易被误用，应优先把约束写进 schema 描述，而不是只留在运行时报错

### `Enabled`

- 表示该 tool 当前是否允许注册后被调用
- 不要把它当作运行时业务开关；它表达的是工具级可用性

### `SecurityLevel`

- 表示调用该 tool 所需的权限级别
- 级别选择应基于工具的真实风险和副作用，而不是实现难度
- 会改动文件、执行命令、访问外部网络或产生明显副作用的 tool，应从风险角度谨慎设级

### `IsConcurrencySafe`

- 只有在并发执行不会引入状态冲突、资源竞争或结果污染时，才设为 `true`
- 只要 tool 会修改共享状态、文件内容或会话数据，默认应保守地设为 `false`

### `Requirements`

- 表达调用前必须满足的上下文前提，例如 `working_dir`、`workspace_root`、网络能力
- `Requirements` 只表达执行前提，不替代 tool 自己的业务校验
- 即使某项 requirement 已声明，tool 仍应在 `Execute()` 中完成必要的边界检查

## `Execute()` 约定

`Execute()` 应保持结构直接、责任清晰。默认流程为：

1. 解码参数
2. 校验输入和运行边界
3. 执行核心动作
4. 组装 `ToolResult`

### 错误分类

- 参数反序列化漂移、内部状态不一致、代码路径失配：`StatusSystemError`
- 用户输入不合法、缺字段、路径越界、格式错误、业务前置条件不满足：`StatusValidationFailed`
- 工具成功执行到业务层，但目标未达成，例如未找到目标、替换未命中、会话不存在：`StatusExecutionError`
- 真实系统故障、依赖故障、外部调用失败、文件或网络操作异常：`StatusSystemError`

### 结果组织

- `Content` 面向调用方和模型，优先保持简洁、直接、可继续推理
- `Metadata` 面向结构化补充信息，不替代 `Content`
- 不要把所有信息都塞进 `Metadata`，导致 `Content` 失去可读性
- 如果执行成功但结果为空，也应返回可理解的结果内容，而不是模糊空串

## 最小测试约定

- 新增本地 tool 时，默认应补对应的定向测试
- 测试至少覆盖：
  - `Metadata()` 的关键字段是否符合预期
  - 正常调用路径
  - 主要校验失败路径
  - 至少一种执行失败或系统失败路径
- 如果 tool 依赖外部系统、网络或进程执行，应优先通过 fake、stub 或可替换依赖做定向测试，而不是把集成路径直接写进单测

## 新增本地 Tool 最小检查清单

- 是否实现了 `Metadata()` 和 `Execute()`
- 是否定义了稳定的工具名
- `Description` 是否写清了用途和使用边界
- `Parameters` 是否为合法 object schema
- 参数字段是否全部带 `description`
- `required` 是否显式声明
- `SecurityLevel`、`IsConcurrencySafe`、`Requirements` 是否按真实风险和前提选择
- `Execute()` 是否区分了解码错误、校验错误、执行错误和系统错误
- `Content` 和 `Metadata` 是否各自承担了清晰职责
- 是否在正确层完成注册和 wiring
- 是否补了与改动范围匹配的定向测试

## 最小验证约定

- tool 层改动完成后，默认至少运行对应包的定向测试
- 如果改动影响了 tool 注册、权限过滤或 schema 校验，应补充运行 `internal/tool` 的 service 相关测试
- 在新增 tool 或调整 metadata 约束时，应更频繁地运行定向测试，不要攒多步后再一次性修复
