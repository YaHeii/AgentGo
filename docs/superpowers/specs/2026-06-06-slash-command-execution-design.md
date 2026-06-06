# Slash 命令执行设计

日期：2026-06-06

## 背景

当前 `internal/ui/model/slash_menu.go` 只负责显示 `/` 命令列表和选择项，`ui.go` 中所有命令执行都会落到 `not implemented`。本次要补齐以下命令：

- `/historySession`：列举历史 session，并复用当前 list 区域让用户用上下键选择
- `/permission`：列举权限等级，并在确认后实时修改 `lifecycle.State.PermissionLevel`
- `/newSession`：创建并切换到新 session
- `/compact`：暂不实现，只给出明确状态提示

## 方案

采用方案 A：复用当前 `SlashMenu` 的 list 作为临时选择面板，不新增独立页面或嵌套 Bubble Tea model。

`SlashMenu` 增加一个轻量模式状态，用来区分当前 list 内容：

- `commands`：默认 `/` 命令列表
- `sessions`：`/historySession` 打开的历史 session 列表
- `permissions`：`/permission` 打开的权限等级列表

UI 顶层仍由 `ui.go` 负责全局按键路由。只要 `SlashMenu` 打开，`↑/↓` 继续交给 list，`Enter` 根据当前模式执行不同动作，`Esc` 关闭 list 并恢复输入态。

## 命令行为

### `/historySession`

选择命令后通过 `appService.ListSessions(ctx)` 异步加载 session 列表，加载结果继续显示在原 slash list 区域。

用户在 session 列表中按 `Enter` 后调用 `appService.SwitchSession(ctx, sessionID)`。实际 sessionID 同步和历史消息加载继续复用现有 `app.EventSession` 事件路径：`SwitchSession` 发布 session event，UI 收到后调用 `loadHistoryCmd`。

空列表或加载失败时不进入异常状态，只关闭 list 并设置 header transient status。

### `/permission`

选择命令后将 list 内容切到固定三项：`safe`、`attention`、`danger`。

用户按 `Enter` 后直接写入 `lifecycle.State.PermissionLevel`。如果 `lifecycle.State == nil`，不创建全局状态，只提示无法修改。

修改完成后关闭 list，并更新 header transient status。header 本身已经从 `lifecycle.State.PermissionLevel` 读取权限显示，因此不需要额外事件。

### `/newSession`

选择命令后通过 `appService.StartNewSession(ctx, title)` 异步创建并切换 session。title 使用一个固定、可读的默认值，例如 `New Session`。

成功后的 session 切换和历史加载继续复用现有 session event 路径。失败时停止在当前 session，并设置 header transient status。

### `/compact`

暂不实现。选择后关闭 list，显示 `not implemented: /compact`。

## 接口边界

UI 的 `appService` 最小扩展为：

- `ListSessions(ctx context.Context) ([]sessioncontract.Session, error)`
- `SwitchSession(ctx context.Context, sessionID string) error`
- `StartNewSession(ctx context.Context, title string) error`

这些方法已经由 `app.APPService` 提供，不需要绕过 app 直接依赖 session service。

权限修改是 UI 对进程级 runtime state 的直接操作，不经过 app。原因是 `lifecycle.State` 当前设计就是公开可写的进程态，header 和工具权限读取都基于它。

## 测试策略

优先补 `internal/ui/model` 的定向测试：

- `/historySession` 执行后加载 session 列表，并且上下键/Enter 能选择并调用 `SwitchSession`
- `/permission` 执行后显示权限列表，Enter 后修改 `lifecycle.State.PermissionLevel`
- `/newSession` 执行后调用 `StartNewSession`
- `/compact` 保持暂未实现提示

实现后运行：

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -count=1
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./... -count=1
```

## 非目标

- 不新增独立 history 页面
- 不实现 compact
- 不新增权限持久化
- 不改变 session event 的现有加载链路
