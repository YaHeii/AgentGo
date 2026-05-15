package lifecycle

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/spf13/viper"
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

// TODO:Move some func from supervisor to here
// func Bootstrap(ctx context.Context, configDir string, _ string) (Runtime, error) {

// }

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
