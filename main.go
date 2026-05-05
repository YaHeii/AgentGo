package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/app"
	cmd "github.com/YaHeii/agentGo/internal/cmd"
	"github.com/YaHeii/agentGo/internal/db"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/utils"
)

func main() {
	cfg, err := utils.LoadConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	providerCfg, err := cmd.ProviderConfigFromAppConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	llm, err := provideropenai.New(providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init provider: %v\n", err)
		os.Exit(1)
	}

	st, err := db.Open("agentgo.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	svc := app.NewService(st, llm, nil)

	p := tea.NewProgram(cmd.NewChatModel(svc))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run program: %v\n", err)
		os.Exit(1)
	}
}
