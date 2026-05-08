# agentGo Repository Guidelines

## Communication

- Use Chinese for discussion and Markdown documents by default.
- Prefer plain Markdown for plans, specs, and explanations. Do not introduce
  HTML unless explicitly requested.
- Keep `AGENTS.md` focused on agent collaboration rules. Project background,
  architecture details, and design discussion should live under `docs/`.

## Workflow

- For architecture changes, boundary refactors, or non-trivial feature work,
  write a Chinese spec first under `docs/superpowers/specs/`.
- After the spec is approved, write an implementation plan under
  `docs/superpowers/plans/` before changing code.
- Once the user approves the plan, move directly into implementation instead of
  reopening the same design discussion.

## Development

- Use TDD by default. Start from a failing test when adding or changing
  behavior.
- Use `apply_patch` for manual file edits.
- Format Go code after edits with `gofmt -w` or an approved formatter.
- Run targeted tests for the packages you touched before finishing.

## Docs And References

- Use `docs/Crush_AGENTS.md` as a reference source, not as a template to copy
  blindly.
- When discussion converges, persist the result as a Markdown document instead
  of leaving the decision only in chat history.


**NEED TO UPDATE**
## Module spec
session 只负责“这场对话是谁、何时创建、当前标题是什么、最近一次活跃是什么、它和别的 session 是什么关系”。

Create / Get / GetLast / List / Rename / Delete
未来可放 ParentSessionID、SummaryMessageID、usage 聚合这类“会话级元数据”
发布 session.Event
不返回 []message.Message
不理解 message 的 Parts / Status / Delta / ToolCall
message 只负责“这场对话里说了什么、消息怎么演进、如何流式更新”。

Create / Update / Get / List / Delete
rich message DTO：Kind / Origin / Status / Parts / Flags
message 流式事件：Created / Delta / Completed / Failed / Cancelled
必须带 SessionID，因为 message 从属于 session
不负责创建 session、选 active session、改 session title
agent/query 只负责“一轮 query 如何驱动 message 演进”。

接收 sessionID
从 message.Service 取历史、创建 user/assistant message、处理 provider stream
查询结束后，如需更新标题/usage，再调用 session.Service
不自己持久化 session
app 只负责编排。

调 session.Service 决定当前 session
调 message.Service 装载历史
订阅 session/message/agent 事件
汇聚成统一 app.Event 给 UI