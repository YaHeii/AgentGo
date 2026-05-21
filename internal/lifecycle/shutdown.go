package lifecycle

import "context"

//TODO:
// mutiple signal detect:
//	SIGINT: Ctrl+C中断信号，调用 gracefulShutdown(0)
// SIGTERM: 终止信号，调用 gracefulShutdown(143)
// SIGHUP: 挂断信号（非Windows），调用 gracefulShutdown(129)
// 孤儿进程检测: 定期检查TTY有效性，自动退出

// Ensure terminal status is restored
// Empty the buffer zone
// Shutdown LSP server
// shutdown MCP server

func (r Runtime) GracefulShutdown(ctx context.Context) error {
	if r.closeFn == nil {
		return nil
	}

	return r.closeFn(ctx)
}

func (s *Supervisor) Close(_ context.Context) error {
	if s == nil {
		return nil
	}
	for _, client := range s.mcpClients {
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil {
			return err
		}
	}
	s.mcpClients = nil
	return nil
}
