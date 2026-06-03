package log

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap/zapcore"
)

const (
	DefaultFileName = "agentgo.log"
	defaultLogDir   = "logs"
	defaultLogLevel = "info"
)

type Config struct {
	Environment string
	LogDir      string
	LogLevel    string
	ProjectRoot string
}

type ResolvedConfig struct {
	Environment    string
	LogDir         string
	LogLevel       string
	FilePath       string
	ConsoleEnabled bool
	Level          zapcore.Level
}

func ResolveConfig(cfg Config) (ResolvedConfig, error) {
	environment := strings.ToLower(strings.TrimSpace(cfg.Environment))
	switch environment {
	case "development", "release":
	default:
		return ResolvedConfig{}, fmt.Errorf("log: invalid environment %q", cfg.Environment)
	}

	logLevel := strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if logLevel == "" {
		logLevel = defaultLogLevel
	}

	level, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		return ResolvedConfig{}, fmt.Errorf("log: invalid level %q: %w", cfg.LogLevel, err)
	}

	logDir := strings.TrimSpace(cfg.LogDir)
	if logDir == "" {
		root := strings.TrimSpace(cfg.ProjectRoot)
		if root == "" {
			logDir = defaultLogDir
		} else {
			logDir = filepath.Join(root, defaultLogDir)
		}
	}

	return ResolvedConfig{
		Environment:    environment,
		LogDir:         logDir,
		LogLevel:       logLevel,
		FilePath:       filepath.Join(logDir, DefaultFileName),
		ConsoleEnabled: environment == "development",
		Level:          level,
	}, nil
}
