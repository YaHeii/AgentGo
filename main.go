package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/ui"
)

func main() {
	ctx := context.Background()

	runtime, err := lifecycle.Bootstrap(ctx, ".", "agentgo.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.GracefulShutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "close runtime: %v\n", err)
		}
	}()

	p := tea.NewProgram(ui.NewRootModel(ui.NewChatService(runtime.App)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run program: %v\n", err)
		os.Exit(1)
	}
}
