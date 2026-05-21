package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFileName = "MCP.json"

type FileConfig struct {
	Servers []Config `json:"servers"`
}

func LoadConfigFile(configDir string) (FileConfig, error) {
	path := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileConfig{}, nil
		}
		return FileConfig{}, err
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, server := range cfg.Servers {
		if err := validateConfig(server); err != nil {
			return FileConfig{}, err
		}
	}
	return cfg, nil
}
