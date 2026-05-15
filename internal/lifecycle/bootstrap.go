package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/YaHeii/agentGo/internal/agent"
	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/tool"
	"github.com/spf13/viper"
)

var (
	State *GlobalState
)

type Config struct {
	Environment   string `mapstructure:"ENVIRONMENT"`
	BaseURL       string `mapstructure:"BASE_URL"`
	APIKey        string `mapstructure:"API_KEY"`
	Model         string `mapstructure:"MODEL"`
	MaxToken      int64  `mapstructure:"MAX_TOKENS"`
	ContextWindow int64  `mapstructure:"CONTEXT_WINDOW"`
	MaxTurn       int64  `mapstructure:"MAX_TURN"`
}

type Runtime struct {
	App     *app.APPService
	closeFn func(context.Context) error
}

func ProviderConfigFromAppConfig(cfg Config) (provider.Config, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return provider.Config{}, errors.New("API_KEY is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return provider.Config{}, errors.New("MODEL is required")
	}

	return provider.Config{
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		APIKey:  strings.TrimSpace(cfg.APIKey),
		Model:   strings.TrimSpace(cfg.Model),
	}, nil
}
func Bootstrap(ctx context.Context, configDir string, databasePath string) (Runtime, error) {
	cfg, err := LoadConfig(configDir)
	if err != nil {
		return Runtime{}, fmt.Errorf("load config: %w", err)
	}
	if _, err := ProviderConfigFromAppConfig(cfg); err != nil {
		return Runtime{}, fmt.Errorf("invalid config: %w", err)
	}
	providerCfg, err := ProviderConfigFromAppConfig(cfg)
	if err != nil {
		return Runtime{}, fmt.Errorf("invalid config: %w", err)
	}

	dispatcher := app.NewDispatcher(128)
	State = &GlobalState{}
	supervisor := NewSupervisor(dispatcher, cfg)
	if err := supervisor.Initialize(ctx); err != nil {
		return Runtime{}, fmt.Errorf("init lifecycle state: %w", err)
	}
	go supervisor.Run(ctx)

	st, err := db.Open(databasePath)
	if err != nil {
		return Runtime{}, fmt.Errorf("open store: %w", err)
	}
	providerClient, err := provider.NewOpenAIClient(providerCfg)
	if err != nil {
		_ = st.Close()
		return Runtime{}, fmt.Errorf("new provider client: %w", err)
	}

	messageSvc := message.NewMessageService(st)
	sessionSvc := session.NewSessionService(st, messageSvc, dispatcher)
	providerSvc := provider.NewProviderService(providerClient, dispatcher)
	runtimeProvider := lifecycleRuntimeProvider{}
	queryApp := app.NewService(sessionSvc, nil, supervisor.toolSvc, dispatcher)
	queryLoop := agent.NewQueryLoop(queryApp, providerSvc, dispatcher)
	queryLoop.SetRuntimeProvider(runtimeProvider)
	agentSvc := queryLoopAdapter{loop: queryLoop}

	dispatcher.Dispatch(app.BaseEvent{
		T: "lifecycle",
		Payload: BootstrapDoneEvent{
			Time:      State.StartTime,
			SessionID: State.SessionID,
		},
	})
	return Runtime{
		App: app.NewService(
			sessionSvc,
			agentSvc,
			supervisor.toolSvc,
			dispatcher,
		),
		closeFn: func(context.Context) error {
			return st.Close()
		},
	}, nil
}

type lifecycleRuntimeProvider struct{}

type queryLoopAdapter struct {
	loop *agent.QueryLoop
}

func (a queryLoopAdapter) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	if a.loop == nil {
		return errors.New("lifecycle: query loop is required")
	}
	_, err := a.loop.RunQuery(ctx, sessionID, prompt)
	return err
}

func (lifecycleRuntimeProvider) Snapshot() agentcontract.RuntimeSnapshot {
	state := GetState()
	return agentcontract.RuntimeSnapshot{
		AppVersion:      state.AppVersion,
		ProjectRoot:     state.ProjectRoot,
		Cwd:             state.Cwd,
		PermissionLevel: tool.SecurityLevel(state.PermissionLevel),
		ModelLimit:      state.ModelLimit,
		Model:           state.Model,
		Temperature:     state.Temperature,
	}
}

func (lifecycleRuntimeProvider) TokenizerForModel(model string) (agentcontract.TokenEncoder, error) {
	return TokenizerForModel(model)
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	v := viper.New()
	v.AddConfigPath(path)
	v.SetConfigName("app")
	v.SetConfigType("env")

	v.AutomaticEnv()

	err = v.ReadInConfig()
	if err != nil {
		return
	}

	err = v.Unmarshal(&config)
	return
}
