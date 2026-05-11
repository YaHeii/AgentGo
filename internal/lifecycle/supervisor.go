package lifecycle

// todo:

// #### A. 持续发现 (Continuous Discovery)

// 持续发现mcp工具和本地tool

// #### B. Token 与上下文监控 (Monitoring)

// 工具执行时的 Token 监控不应是“全生命周期”的，而应是“单次调用（Request-scoped）”的。

// ```go
// func (s *ToolSupervisor) Execute(ctx context.Context, id string, args json.RawMessage) (Result, error) {
//     // 1. 获取带有 Token 预算的子 Context
//     // 假设你在 context 中注入了当前 Session 的 Token 限制
//     execCtx, cancel := context.WithTimeout(ctx, s.maxExecutionTime)
//     defer cancel()

//     // 2. 监控协程：实时计算消耗
//     go func() {
//         for {
//             select {
//             case <-execCtx.Done():
//                 return
//             case <-s.tokenAlert: 
//                 // 如果检测到 Token 异常溢出（例如模型陷入死循环不断输出）
//                 cancel() // 强制终止底层进程
//             }
//         }
//     }()

//     return s.registry.Get(id).Execute(execCtx, args)
// }

// ```

// 为了实现“不符合则通过 Context 取消进程”，你需要实现一个 **`Execution Guard`（执行卫士）**：

// 1. **进程绑定**：对于 `bashtool`，确保 `os/exec.CommandContext` 使用的是受控的 `ctx`。当 `ctx` 被取消时，Go 会自动发送 `SIGKILL` 给子进程。
// 2. **Token 熔断器**：在 `ToolResult` 回传时，记录每一轮的 Token 消耗。如果单次工具调用产生的日志/数据量超过阈值（例如 1MB），立即触发 `cancel()`。
// 3. **副作用回滚**：如果进程因为不合规被强杀，`Supervisor` 应该尝试清理现场（如删除临时文件）。

// ---