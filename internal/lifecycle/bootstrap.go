package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/message"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/segmentio/ksuid"
	"github.com/spf13/viper"
)

var (
	State *GlobalState
)

type Runtime struct {
	App     *app.APPService
	closeFn func(context.Context) error
}

func ProviderConfigFromAppConfig(cfg Config) (provideropenai.Config, error) {
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
	cfg, err := LoadConfig(configDir)
	if err != nil {
		return Runtime{}, fmt.Errorf("load config: %w", err)
	}
	providerCfg, err := ProviderConfigFromAppConfig(cfg)
	if err != nil {
		return Runtime{}, fmt.Errorf("invalid config: %w", err)
	}
	state, err := LoadState()
	if err != nil {
		return Runtime{}, fmt.Errorf("init lifecycle state: %w", err)
	}
	State = &state
	dispatcher := app.NewDispatcher(128)
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

	messageSvc := message.NewMessageService(st, dispatcher)
	sessionSvc := session.NewSessionService(st, messageSvc, dispatcher)
	agentSvc := agent.NewQueryLoop(sessionSvc, llm, dispatcher)

	dispatcher.Dispatch(app.BaseEvent{
		T: "lifecycle",
		Payload: BootstrapDoneEvent{
			time:      state.StartTime,
			sessionID: state.SessionID,
		},
	})
	return Runtime{
		App: app.NewService(
			sessionSvc,
			agentSvc,
			dispatcher,
		),
		closeFn: func(context.Context) error {
			return st.Close()
		},
	}, nil
}

type Config struct {
	Environment string `mapstructure:"ENVIRONMENT"`
	BaseURL     string `mapstructure:"BASE_URL"`
	APIKey      string `mapstructure:"API_KEY"`
	Model       string `mapstructure:"MODEL"`
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

// XXX: Optimize error handling
// XXX: Enriched fields
func LoadState() (state GlobalState, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return GlobalState{}, err
	}

	projectRoot, err := filepath.Abs(cwd)
	if err != nil {
		return GlobalState{}, err
	}

	startTime := time.Now().UTC()
	sessionID, err := ksuid.NewRandomWithTime(startTime) //Ksuid uses time information for easy sorting.
	if err != nil {
		return GlobalState{}, fmt.Errorf("generate session id: %w", err)
	}

	state = GlobalState{
		AppVersion:  "0.0.1",
		StartTime:   startTime.Format(time.RFC3339Nano),
		Cwd:         cwd,
		ProjectRoot: projectRoot,
		SessionID:   sessionID.String(),
	}

	return state, nil
}
