package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/ui"
)

func main() {
	ctx := context.Background()

	cfg, err := lifecycle.LoadConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	dispatcher := app.NewDispatcher(128)
	lifecycle.State = &lifecycle.GlobalState{}
	supervisor := lifecycle.NewSupervisor(dispatcher, cfg)
	if err := supervisor.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "init lifecycle state: %v\n", err)
		os.Exit(1)
	}
	lifecycle.CurrentSupervisor = supervisor
	go supervisor.Run(ctx)

	st, err := db.Open("agentgo.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	providerClient, err := provider.NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	if err != nil {
		_ = st.Close()
		fmt.Fprintf(os.Stderr, "new provider client: %v\n", err)
		os.Exit(1)
	}

	messageSvc := message.NewMessageService(st)
	sessionSvc := session.NewSessionService(st, messageSvc, dispatcher)
	providerSvc := provider.NewProviderService(providerClient, dispatcher)
	toolSvc := supervisor.ToolService()
	queryApp := app.NewService(sessionSvc, nil, toolSvc, dispatcher)
	//TOFIX: ui should do this, not main
	queryLoop := agent.NewQueryLoop(queryApp, providerSvc, dispatcher)
	agentSvc := agent.NewService(queryLoop)
	appSvc := app.NewService(sessionSvc, agentSvc, toolSvc, dispatcher)
	defer func() {
		if err := st.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close runtime: %v\n", err)
		}
	}()

	p := tea.NewProgram(ui.NewRootModel(ui.NewChatService(appSvc)))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run program: %v\n", err)
		os.Exit(1)
	}
}
