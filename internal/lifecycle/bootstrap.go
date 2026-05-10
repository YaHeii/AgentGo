package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/message"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/utils"
)

type Runtime struct {
	App *app.APPService
	Bus bus.Bus[app.Event]

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
func Bootstrap(ctx context.Context, configDir string, databasePath string) (Runtime, error) {
	cfg, err := utils.LoadConfig(configDir)
	if err != nil {
		return Runtime{}, fmt.Errorf("load config: %w", err)
	}

	providerCfg, err := ProviderConfigFromAppConfig(cfg)
	if err != nil {
		return Runtime{}, fmt.Errorf("invalid config: %w", err)
	}

	runtimeState, err := newBootstrapState()
	if err != nil {
		return Runtime{}, fmt.Errorf("init lifecycle state: %w", err)
	}
	Global = runtimeState

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

	appBus := bus.NewBus[app.Event](128)
	messageSvc := message.NewMessageService(st)
	sessionSvc := session.NewSessionService(st, messageSvc, nil)
	agentSvc := agent.NewQueryLoop(sessionSvc, llm)

	go func() {
		for evt := range sessionSvc.Events() {
			appBus.Publish(evt)
		}
	}()
	go func() {
		for evt := range messageSvc.Events() {
			appBus.Publish(evt)
		}
	}()
	go func() {
		for evt := range agentSvc.Events() {
			appBus.Publish(evt)
		}
	}()

	return Runtime{
		App: app.NewService(app.Dependencies{
			Sessions: sessionSvc,
			Agent:    agentSvc,
			Bus:      appBus,
		}),
		Bus: appBus,
		closeFn: func(context.Context) error {
			return st.Close()
		},
	}, nil
}

func newBootstrapState() (*GlobalState, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	projectRoot, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}

	state := &GlobalState{
		RuntimeConfig: map[string]interface{}{},
	}
	state.SetCurrentCWD(cwd)
	state.SetProjectRoot(projectRoot)
	state.SetSessionSeed("", "")

	return state, nil
}

// TODO: Detect local runtime environment and workspace prerequisites during bootstrap.
// TODO: Assemble tool registry during bootstrap.
// TODO: Assemble MCP clients or connectors during bootstrap.
// TODO: Assemble skill registry or skill loader during bootstrap.
