# 上下文感知压缩策略设计

日期：2026-09-03

## 1. 背景

AgentGo 当前已经具备基于 token 的上下文预算控制：

- 使用 `tiktoken` 估算请求 token；
- 以 `MAX_TOKENS` 作为总上下文窗口；
- 达到窗口 90% 时触发压缩；
- 生成一条 `PartTypeSummary` 消息；
- 将压缩消息标记为 `IsCompactSummary=true`。

当前摘要主要是确定性的文本拼接和截断，无法判断内容是否与当前任务相关，也无法稳定保留关键决策、约束和验证状态。

本 spec 定义第二阶段的上下文感知压缩策略。

## 2. 目标

### 2.1 必须实现

- 在上下文使用率达到 `MAX_TOKENS * 90%` 时触发自动压缩；
- 优先批量压缩尚未压缩的 tool result；
- 摘要生成时注入当前任务和已有上下文；
- 生成一条可持久化的摘要消息；
- 将摘要消息的 `IsCompactSummary` 设置为 `true`；
- 防止同一批历史被重复压缩；
- 保留关键决策、约束、文件变更、验证状态和未完成事项；
- 保留 URL、文件路径、命令、ID、hash、日期等可回溯标识符；
- 压缩失败时回退到安全的 token 裁剪策略。

### 2.2 非目标

- 本阶段不删除原始消息；
- 本阶段不改变 `messages` 表结构；
- 本阶段不要求实现多级摘要树；
- 本阶段不要求实现跨 session 共享记忆；
- 本阶段不要求摘要完全替代最近的完整对话；
- 本阶段不强制实现 `/compact`，但手动命令应复用同一压缩服务。

## 3. 核心原则

### 3.1 压缩是任务相关的

压缩器不能只按照字符数或消息数量截断内容。摘要请求必须包含：

```text
当前用户任务
当前已有摘要
最近仍保留的关键上下文
待压缩的原始 tool result
```

压缩模型只提炼与当前任务相关的信息。

### 3.2 优先压缩工具结果

压缩优先级：

1. 未压缩的 tool result；
2. 过早的工具调用及其结果；
3. 已完成且不再需要原文的辅助信息；
4. 普通 assistant 解释文本。

默认不压缩：

- 当前用户消息；
- 当前任务目标；
- system prompt；
- 最近一轮尚未完成的 tool call/tool result；
- 关键 assistant 决策；
- 尚未解决的错误和 TODO。

### 3.3 保留事实，不保留噪声

摘要必须优先保留：

- 用户目标和成功标准；
- 架构决策；
- 关键约束；
- 修改过的文件；
- 重要代码位置；
- 已执行命令；
- 测试结果；
- 当前错误；
- 未完成任务；
- 回滚或恢复信息；
- URL、文件路径、ID、hash、日期等标识符。

可以丢弃：

- 重复的工具输出；
- 大段未使用的源码；
- 已经被后续结果覆盖的中间状态；
- 不影响当前任务的日志噪声；
- 已经确认无关的搜索结果。

## 4. 触发条件

### 4.1 Token 阈值

完整 provider 请求的估算 token 达到以下阈值时触发压缩：

```text
compactThreshold = MAX_TOKENS * 90%
```

估算范围包括：

- system prompt；
- 当前已有摘要；
- 用户消息；
- assistant 消息；
- thinking；
- tool call；
- tool result；
- tool definitions；
- 消息协议开销；
- 安全余量。

压缩触发判断必须基于即将发送的完整请求，而不是只计算历史正文。

### 4.2 压缩目标

压缩完成后，目标是将请求控制在：

```text
compressedTarget = MAX_TOKENS * 70% ~ 80%
```

推荐默认目标为 `70%`，为后续工具调用和 assistant 输出留下增长空间。

摘要自身建议不超过总窗口的 `10%`。

## 5. 压缩单元

### 5.1 工具交互单元

以下消息必须作为一个不可拆分单元处理：

```text
assistant(tool call)
system(tool result)
```

同一 assistant 消息中的多个 tool call 与对应结果也应保持配对。

禁止产生：

- 孤立 tool result；
- 没有结果的历史 tool call；
- 只保留工具输出而丢失调用意图；
- 只保留调用意图而丢失关键执行结果。

### 5.2 普通对话单元

普通对话可以按以下逻辑分组：

```text
user
assistant
```

当前用户消息和最近一轮交互优先保留为完整消息。

## 6. 压缩流程

### 6.1 总体流程

```text
加载 session 历史
  -> 找到最新 IsCompactSummary=true 的摘要
  -> 只处理摘要之后的新增历史
  -> 估算完整请求 token
  -> 未达到 90%：按 token 预算选择历史
  -> 达到 90%：收集未压缩的可压缩消息
  -> 批量调用压缩模型
  -> 校验摘要格式和 token 大小
  -> 持久化摘要消息
  -> 使用摘要 + 最近完整消息构造请求
```

### 6.2 收集压缩输入

压缩器需要选择一批历史消息，优先选择：

- 时间较早；
- 已完成；
- 已有明确结果；
- 尚未设置 `IsCompactSummary`；
- 不属于当前未完成工具链。

如果只压缩部分历史，压缩范围必须是完整的消息单元。

### 6.3 压缩请求输入

压缩模型的请求至少包含：

```text
Current task:
当前用户任务

Existing summary:
已有压缩摘要

Recent context:
最近仍保留的完整上下文

Messages to compress:
待压缩的 tool result 和关联交互
```

### 6.4 摘要输出格式

摘要应使用稳定的 Markdown 结构：

```markdown
# Task

## Goal
- 当前用户目标

## Decisions
- 已确认的架构和实现决策

## Changes
- 文件路径：关键变更

## Tool Results
- 工具名称：关键结论
- 来源：URL、文件路径或其他引用

## Verification
- 已执行的命令
- 测试结果

## Open Items
- 未完成事项
- 当前错误

## Identifiers
- 需要原样保留的 ID、hash、日期、命令或路径
```

如果某个章节没有内容，可以省略，但 `Goal`、`Open Items` 和 `Verification` 不应被无故删除。

### 6.5 摘要持久化

生成摘要后，通过现有 message service 保存为一条新消息：

```go
message.CreateMessageParams{
    SessionID:        sessionID,
    Kind:             message.KindSystem,
    IsCompactSummary: true,
    Parts: []message.Part{
        {
            Type: message.PartTypeSummary,
            Text:  summaryText,
        },
    },
}
```

摘要消息必须满足：

- `IsCompactSummary == true`；
- `Kind == KindSystem`；
- 包含 `PartTypeSummary`；
- 内容不超过摘要 token 预算；
- 能被后续历史加载识别为压缩锚点。

原始消息默认保留在 `messages` 表中，不在本阶段删除。

## 7. 重复压缩控制

### 7.1 压缩标记

`IsCompactSummary=true` 仅用于标记摘要消息，不应标记原始 tool result。

已经被摘要覆盖的历史不能在下一次压缩中再次作为新的压缩输入。

### 7.2 摘要锚点

历史加载时找到最新的 `IsCompactSummary=true` 消息，默认只将以下内容放入后续上下文：

```text
最新摘要
+ 摘要之后产生的新消息
```

摘要必须覆盖该压缩点之前需要继续使用的关键信息。

### 7.3 幂等性

同一 session 的同一批历史在成功持久化摘要后，不得重复创建等价摘要。

如果系统未来引入压缩批次 ID，可以使用：

```text
session_id
compression_range
source_message_ids
```

实现更严格的幂等校验。

## 8. 失败回退

### 8.1 压缩模型调用失败

压缩模型调用失败时：

1. 记录压缩失败；
2. 不写入半成品摘要；
3. 使用 token 预算选择最近完整消息；
4. 必要时截断超大的 tool result；
5. 如果仍然无法构造合法请求，则返回明确错误。

### 8.2 摘要格式无效

以下情况视为摘要无效：

- 空摘要；
- 超过摘要 token 上限；
- 丢失当前任务目标；
- 丢失关键引用；
- 产生无法解析的结构；
- 内容包含与输入无关的大段原文。

无效摘要不得持久化。

### 8.3 连续失败保护

同一 session 连续压缩失败时，应避免每次请求都重复调用压缩模型。可以使用短期失败标记：

```text
session_id
last_compaction_failure
failure_count
retry_after
```

具体持久化方式后续确定。

## 9. 服务边界

建议新增 agent 内部压缩服务，不将压缩细节放入 `app` facade：

```go
type ContextCompactor interface {
    Compact(ctx context.Context, request CompactRequest) (CompactResult, error)
}
```

`CompactRequest` 至少包含：

```go
type CompactRequest struct {
    SessionID       string
    CurrentTask     string
    ExistingSummary string
    RecentContext   []message.Message
    Candidates      []message.Message
    Model           string
    TokenBudget     int
}
```

`CompactResult` 至少包含：

```go
type CompactResult struct {
    SummaryText string
    SourceIDs   []string
    TokenCount  int
}
```

压缩器负责：

- 构造压缩模型请求；
- 校验返回摘要；
- 计算摘要 token；
- 返回摘要内容和来源消息 ID。

`QueryLoop` 负责：

- 判断是否触发压缩；
- 选择压缩候选；
- 调用 compactor；
- 通过 app/message service 持久化摘要；
- 重新构造 provider 请求。

## 10. Provider 选择

默认优先使用独立的压缩模型，避免压缩请求消耗主任务的上下文和输出预算。

当前实现复用已有 provider，并使用 `internal/agent/template/compaction.md.tpl` 渲染独立的压缩 prompt。压缩请求只包含一个临时 user message，不写入普通对话历史。

如果未来接入独立模型，必须：

- 使用独立的压缩请求；
- 不将压缩请求写入普通对话历史；
- 不把压缩模型的输出当成 assistant 任务回复；
- 正确统计压缩请求成本和 token。

provider 调用失败或返回空文本时，使用确定性的 transcript compaction 作为 fallback。

## 11. 观测指标

至少记录以下指标：

- 压缩触发次数；
- 压缩成功次数；
- 压缩失败次数；
- 压缩前 token；
- 压缩后 token；
- 摘要 token；
- 被压缩消息数量；
- 被压缩 tool result 数量；
- 回退裁剪次数；
- 摘要模型耗时；
- 摘要模型 token 消耗。

`lifecycle.State` 中的上下文指标需要区分：

- provider 实际 prompt token；
- 压缩前估算 token；
- 压缩后估算 token；
- 摘要 token；
- 回退裁剪状态。

## 12. 测试要求

### 12.1 单元测试

- 90% 阈值触发压缩；
- 低于 90% 不触发压缩；
- 只选择未压缩的 tool result；
- tool call 和 tool result 保持配对；
- 摘要消息设置 `IsCompactSummary=true`；
- 摘要使用 `PartTypeSummary`；
- 摘要 token 不超过预算；
- 摘要按当前任务生成；
- 摘要格式无效时不持久化；
- tokenizer 失败时正确回退或返回错误。

### 12.2 集成测试

- 压缩摘要通过 message service 写入 `messages` 表；
- 下次加载 session 时从最新摘要继续；
- 原始消息仍然存在；
- 同一批历史不会重复生成摘要；
- 压缩失败后可以使用 token 裁剪继续请求；
- 工具循环中的压缩不会破坏 provider 消息协议。

### 12.3 回归验证

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/agent ./internal/provider ./internal/message ./internal/session ./internal/lifecycle -count=1
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./... -count=1
```

## 13. 实施顺序

1. 定义 `ContextCompactor`、`CompactRequest` 和 `CompactResult`。
2. 将当前确定性摘要逻辑迁移为 fallback compactor。
3. 增加压缩候选选择器，只选择未压缩且已完成的工具交互。
4. 增加压缩模型 provider 请求。
5. 增加摘要格式校验和 token 限制。
6. 接入 `QueryLoop` 的 90% 自动触发流程。
7. 验证摘要持久化和摘要锚点恢复。
8. 增加压缩失败回退和连续失败保护。
9. 最后再考虑 `/compact` 和 `summary_message_id`。

## 14. 设计结论

AgentGo 第二阶段采用：

```text
90% token 阈值
  -> 批量选择旧的未压缩 tool result
  -> 结合当前任务生成上下文感知摘要
  -> 保留关键决策、约束、变更、验证状态和引用
  -> 保存为一条 IsCompactSummary=true 的 summary message
  -> 摘要失败则回退到 token 裁剪
```

当前确定性文本截断实现保留为最终兜底，不作为主压缩策略。
