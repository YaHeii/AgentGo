package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/db"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/utils"
)

type Runtime struct {
	App *app.Service

	closeFn func(context.Context) error
}

func ProviderConfigFromAppConfig(cfg utils.Config) (provideropenai.Config, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return provideropenai.Config{}, errors.New("API_KEY is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return provideropenai.Config{}, errors.New("MODEL is required")
	}

	return provideropenai.Config{
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		APIKey:  strings.TrimSpace(cfg.APIKey),
		Model:   strings.TrimSpace(cfg.Model),
	}, nil
}

//TODO: initialize the state.go
func Bootstrap(ctx context.Context, configDir string, databasePath string) (Runtime, error) {
	cfg, err := utils.LoadConfig(configDir)
	if err != nil {
		return Runtime{}, fmt.Errorf("load config: %w", err)
	}

	providerCfg, err := ProviderConfigFromAppConfig(cfg)
	if err != nil {
		return Runtime{}, fmt.Errorf("invalid config: %w", err)
	}

	// TODO: Detect local runtime environment and workspace prerequisites during bootstrap.
	// TODO: Assemble tool registry during bootstrap.
	// TODO: Assemble MCP clients or connectors during bootstrap.
	// TODO: Assemble skill registry or skill loader during bootstrap.

	llm, err := provideropenai.New(providerCfg)
	if err != nil {
		return Runtime{}, fmt.Errorf("init provider: %w", err)
	}

	st, err := db.Open(databasePath)
	if err != nil {
		return Runtime{}, fmt.Errorf("open store: %w", err)
	}

	return Runtime{
		App: app.NewService(st, llm, nil),
		closeFn: func(context.Context) error {
			return st.Close()
		},
	}, nil
}

// TODO: Detect local runtime environment and workspace prerequisites during bootstrap.
// TODO: Assemble tool registry during bootstrap.
// TODO: Assemble MCP clients or connectors during bootstrap.
// TODO: Assemble skill registry or skill loader during bootstrap.
