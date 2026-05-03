package main

import (
	"fmt"

	"os"

	tea "charm.land/bubbletea/v2"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/utils"
	cmd "github.com/YaHeii/agentGo/internal/cmd"
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

	p := tea.NewProgram(cmd.NewChatModel(llm))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run program: %v\n", err)
		os.Exit(1)
	}
}
