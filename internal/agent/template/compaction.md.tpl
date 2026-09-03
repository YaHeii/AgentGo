# Context Compaction Task

你负责压缩 agent 对话上下文。请只保留与当前任务相关的信息，不要输出解释性前言。

## Current Task

{{.CurrentTask}}

## Existing Summary

{{if .ExistingSummary}}{{.ExistingSummary}}{{else}}(none){{end}}

## Recent Context

{{if .RecentContext}}{{range .RecentContext}}
### {{.Role}}

{{.Content}}
{{end}}{{else}}(none){{end}}

## Messages To Compress

{{if .Candidates}}{{range .Candidates}}
### {{.Role}}

{{.Content}}
{{end}}{{else}}(none){{end}}

## Output Requirements

输出一份简洁、可继续执行任务的 Markdown 摘要，使用以下章节：

# Task

## Goal
保留当前任务目标和成功标准。

## Decisions
保留已经确认的架构、实现和约束决策。

## Changes
保留修改过的文件、关键代码位置和重要变更。

## Tool Results
只保留工具执行的关键结论，并保留 URL、文件路径、命令、ID、hash 和日期等标识符。

## Verification
保留已经执行的测试、命令和验证结果。

## Open Items
保留未完成事项、当前错误和后续动作。

不要捏造输入中不存在的事实。不要大段复制原始工具输出。摘要长度不得超过 {{.TokenBudget}} tokens。
