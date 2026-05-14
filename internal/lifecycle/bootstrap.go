package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
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

	messageSvc := message.NewMessageService(st, dispatcher)
	sessionSvc := session.NewSessionService(st, messageSvc, dispatcher)

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
			newBootstrapAgentService(),
			dispatcher,
		),
		closeFn: func(context.Context) error {
			return st.Close()
		},
	}, nil
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

type bootstrapAgentService struct{}

func newBootstrapAgentService() *bootstrapAgentService {
	return &bootstrapAgentService{}
}

func (s *bootstrapAgentService) RunPrompt(_ context.Context, _ string, _ string) error {
	return errors.New("lifecycle bootstrap agent service is not wired to runtime agent layer")
}
