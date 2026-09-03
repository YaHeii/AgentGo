# Token 上下文窗口与自动压缩设计

日期：2026-09-03

## 配置语义

`MAX_TOKENS` 表示一次 provider 请求允许使用的总上下文 token 窗口。

`CONTEXT_WINDOW` 仅作为兼容旧配置的 fallback。当 `MAX_TOKENS > 0` 时，优先使用 `MAX_TOKENS`。

当前阶段不再把 `MAX_TOKENS` 当作模型输出 token 上限。输出字段保留在 provider contract 中，但默认不由该配置填充。

## Token 计算

请求预算使用 `tiktoken` 估算，计算范围包括：

- system prompt；
- 历史消息文本；
- thinking 内容；
- tool call 名称和参数；
- tool result；
- summary 内容；
- 工具定义及参数 schema；
- 每条消息的保守协议开销。

上下文预算预留固定的 256 token 安全余量。

```text
压缩阈值 = MAX_TOKENS * 90%
历史预算 = MAX_TOKENS - 固定请求开销 - 安全余量
```

## 自动压缩

当完整请求的估算 token 达到 `MAX_TOKENS * 90%` 时：

1. 从历史中识别最近的压缩摘要。
2. 只在最新摘要之后继续处理历史。
3. 按完整对话单元保留最近消息。
4. 将本次压缩前的有效历史生成一个摘要消息。
5. 将摘要控制在总窗口的 10% 以内。
6. 持久化摘要消息：

```go
Kind:             message.KindSystem
IsCompactSummary: true
PartType:         message.PartTypeSummary
```

7. 本次请求使用摘要消息和最近保留的完整消息。

原始消息不会被删除。摘要消息晚于原始历史写入，因此摘要必须覆盖压缩前的全部有效历史，避免下次从摘要位置恢复时丢失被保留的近期消息。

## 当前压缩形式

当前实现是确定性的 transcript compaction：按最近消息优先拼接摘要内容，再按 token 上限截断。

它不是 LLM 语义摘要。语义摘要、摘要合并、摘要游标和 `/compact` 仍属于后续阶段。

## 工具消息完整性

assistant tool call 与相邻的对应 tool result 组成不可拆分的历史单元。

历史选择不能产生孤立的 tool result，也不能保留 tool call 而丢弃其结果。

## 未决事项

- 是否为 session 更新 `summary_message_id`；
- 是否删除或归档已被摘要覆盖的旧消息；
- 是否使用独立模型生成语义摘要；
- `/compact` 是否调用同一套自动压缩服务；
- 超大单条 tool result 是否支持局部截断或独立摘要。
