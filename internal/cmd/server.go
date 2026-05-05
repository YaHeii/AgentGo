package cmd

import (
	"errors"
	"strings"

	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/utils"
)

//TODO: add Oauth
//WIP: As the minimal bootstrap layer:
// Loads configuration
// Connects to the database
// Creates the workspace (local or remote)
// Initializes the service and event systems

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
