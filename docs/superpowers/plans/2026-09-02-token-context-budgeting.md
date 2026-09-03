# 基于 Token 的上下文预算实施计划

> 状态：第一阶段已实现，第二阶段待设计  
> 日期：2026-09-03

## 目标

将当前 AgentGo 的上下文控制从“固定保留最近 20 条消息 + 超限时逐条丢弃”调整为基于 token 预算的请求级上下文选择策略。

第一阶段只解决：

- 使用 `tiktoken` 估算上下文 token；
- 为 system prompt、工具定义和模型输出预留预算；
- 按 token 预算选择历史消息；
- 保持工具调用与工具结果的消息配对；
- 在无法估算或单条消息过大时明确失败。

第一阶段不实现：

- 语义摘要生成；
- `/compact` 命令；
- 删除数据库中的旧消息；
- 修改现有 session/message 数据库结构。

## 当前问题

当前实现位于 `internal/agent/prompt.go`：

- `messageWindow` 硬编码为 `20`；
- token 估算只覆盖历史消息正文；
- 没有计算 system prompt；
- 没有计算工具 schema；
- 没有为输出 token 预留空间；
- 使用 UTF-8 字节长度作为 token limit 的快速判断；
- token 估算失败时静默认为未超限；
- 以单条消息为单位裁剪，可能破坏 tool call/tool result 配对。

已有的 `SummaryMessageID` 暂不参与本阶段；`IsCompactSummary` 和 `PartTypeSummary` 已用于自动压缩摘要消息。

## 设计方案

### 1. 定义请求预算

请求上下文窗口以 `MAX_TOKENS` 为准，达到其 90% 时触发压缩：

```text
压缩触发阈值 = MAX_TOKENS * 90%
历史可用预算 = MAX_TOKENS - system prompt token - tool definitions token - 安全余量
```

`CONTEXT_WINDOW` 仅作为兼容旧配置的 fallback，不再优先于 `MAX_TOKENS`。

触发压缩后生成一个受 token 上限约束的 `PartTypeSummary` 消息，并设置：

```go
IsCompactSummary: true
```

原始历史仍保留在数据库中，后续请求从最新压缩摘要之后继续。

### 2. 统一 token 估算入口

新增一个只负责估算 provider 请求的内部组件或函数，输入应包含：

- system prompt；
- conversation messages；
- tools；
- model；
- context window。

估算至少覆盖：

- 每条消息的文本内容；
- thinking 内容；
- tool call 名称和参数；
- tool result；
- tool 定义和参数 schema；
- system prompt；
- 基本 role/message 开销的保守余量。

估算结果建议使用结构体表达，而不是只返回一个整数：

```go
type ContextBudget struct {
    ModelLimit       int
    FixedTokens      int
    HistoryBudget    int
    EstimatedTokens  int
    SafetyMargin     int
}
```

具体类型名称可以根据现有包结构调整，不要求暴露到 `contract`。

### 3. 按消息单元选择历史

移除或降级 `messageWindow = 20` 的硬限制，改为从最新历史向前选择，直到达到 `HistoryBudget`。

消息选择应遵循：

- 最新用户消息必须保留；
- 保留顺序不能改变；
- 被选择的消息最终仍按时间正序发送；
- assistant tool call 与对应 tool result 作为一个不可拆分单元；
- 如果一个历史单元放不下，则继续向更近的单元选择；
- 不应只保留孤立的 tool result；
- 不应只保留带 tool call 的 assistant 消息而丢失结果。

建议内部使用“对话单元”而不是直接操作 `[]message.Message`：

```text
普通消息单元：
  user / assistant / system

工具消息单元：
  assistant(tool calls) + system(tool results)
```

当前数据库按消息顺序返回历史，因此第一阶段可以基于相邻消息和 tool call ID 做最小配对，不引入新的持久化协议。

### 4. 单条消息和估算失败处理

如果单个不可拆分单元本身超过历史预算：

- tool result：返回受控错误，后续可扩展为工具结果截断；
- 普通用户消息：返回上下文超限错误；
- system prompt：初始化阶段直接返回错误；
- 不支持模型 tokenizer：返回错误，不再静默放行。

错误信息应包含估算 token、预算和消息类型，便于 UI 和日志定位。

### 5. Provider 请求同步

保留 `Request.Context.MaxOutputTokens` 的 provider contract 兼容性，但本阶段不使用 `MAX_TOKENS` 填充该字段。

### 6. 生命周期状态与观测

继续通过现有 agent event 更新 `lifecycle.State`，但要区分：

- 估算的完整请求 token；
- 实际 provider 返回的 prompt token；
- 历史消息 token；
- 固定开销 token；
- 裁剪后的消息数量。

第一阶段至少保证 `EstimatedContextTokens` 表示实际发送请求的估算值，而不是仅表示历史正文 token。

## 实施步骤

### Task 1：补齐预算与估算测试

**涉及文件：**

- `internal/agent/*_test.go`
- `internal/lifecycle/*_test.go`
- 可能新增 `internal/agent/context_budget_test.go`

- [x] 为 system prompt、历史消息、工具 schema 和输出预算编写 token 预算测试。
- [x] 验证 token 预算使用 token 数，而不是字节数。
- [x] 验证 tokenizer 不支持时返回错误。
- [x] 验证固定开销和历史预算计算正确。

验证命令：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/agent ./internal/lifecycle -run 'Test.*(Token|Budget|Context)' -count=1
```

### Task 2：实现完整请求 token 估算

**涉及文件：**

- `internal/agent/prompt.go`
- 可能新增 `internal/agent/context_budget.go`
- `internal/lifecycle/supervisor.go`
- 对应测试文件

- [x] 抽取统一的 token 计算逻辑。
- [x] 纳入 system prompt 和 tools。
- [x] 为消息协议开销增加保守余量。
- [x] 计算可用于历史的剩余预算。
- [x] 让估算错误向上返回。

验证：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/agent ./internal/lifecycle -count=1
```

### Task 3：实现按 token 的历史选择器

**涉及文件：**

- `internal/agent/prompt.go`
- 可能新增 `internal/agent/history_selector.go`
- `internal/agent/*_test.go`

- [x] 移除固定 20 条作为主要限制。
- [x] 从最新消息向前按 token 预算选择。
- [x] 保持消息顺序。
- [x] 保证 tool call/tool result 配对。
- [x] 保证当前用户消息不被历史裁剪逻辑丢失。
- [x] 覆盖空历史、恰好命中预算、超过预算、单元过大等情况。

验证：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/agent -run 'Test.*(History|Message|Tool|Context)' -count=1
```

### Task 4：接入 `MAX_TOKENS` 上下文窗口

**涉及文件：**

- `internal/lifecycle/bootstrap.go`
- `internal/lifecycle/state.go`
- `internal/lifecycle/supervisor.go`
- `internal/agent/prompt.go`
- 相关测试

- [x] 将 `MAX_TOKENS` 作为总上下文窗口。
- [x] 在 90% 阈值触发压缩。
- [x] 为缺省值定义稳定行为。
- [x] 将压缩消息标记为 `IsCompactSummary=true`。

验证：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/lifecycle ./internal/agent ./internal/provider -count=1
```

### Task 5：更新 provider 请求映射

**涉及文件：**

- `internal/provider/client.go`
- `internal/provider/provider_test.go`

- [x] 保留 provider contract 的输出字段兼容性。
- [x] 验证上下文预算逻辑不依赖字节长度。
- [x] 验证压缩消息使用 summary 类型并持久化标记。

验证：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/provider -count=1
```

### Task 6：全量验证与文档

**涉及文件：**

- `docs/superpowers/specs/` 下新增稳定设计结论文档；
- 必要时更新现有配置说明。

- [ ] 运行所有 Go 测试。当前 `internal/tool` 有两个与本次改动无关的既有 grep 测试失败。
- [x] 检查首轮请求和后续工具循环请求的预算行为一致。
- [ ] 检查 UI 显示的估算 token 与 provider 实际 prompt token 不混淆。当前仍需后续完善观测字段。
- [x] 记录第一阶段不包含语义摘要和 `/compact`。

验证命令：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./... -count=1
```

## 第二阶段：语义压缩预留

第一阶段完成后，再单独设计摘要压缩：

```text
旧历史
  -> 压缩模型生成摘要
  -> 持久化 summary message
  -> 更新 session.summary_message_id
  -> 请求使用 summary + 最近完整消息
```

第二阶段需要额外决定：

- 摘要由当前 provider 生成，还是使用独立模型；
- 摘要是否作为普通 system message 或 `PartTypeSummary`；
- 是否保留摘要前的原始消息；
- 多次压缩如何合并；
- `/compact` 是否触发显式压缩；
- 自动压缩和手动压缩是否共用同一服务。

这些内容不放入本次 token budget 第一阶段，避免把“预算控制”和“语义摘要”耦合成一次大改动。

## 待审核决策

1. 是否新增安全余量配置？  
   推荐：第一阶段使用内部常量，后续根据实际 provider usage 调整。

2. 是否立即移除固定 20 条限制？  
   推荐：移除其硬限制，仅保留一个可选的消息数量上限作为防御性上限。

3. 单条超大 tool result 第一阶段如何处理？  
   推荐：先返回明确错误，不在本阶段实现二次摘要。

4. 是否要求第一阶段同时支持 `/compact`？  
   推荐：不要求，继续保持未实现状态。
