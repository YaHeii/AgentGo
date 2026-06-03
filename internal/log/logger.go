package log

import (
	"errors"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxSizeMB  = 10
	maxBackups = 5
	maxAgeDays = 7
)

var (
	globalMu     sync.RWMutex
	globalLogger *zap.Logger
)

func NewLogger(cfg ResolvedConfig) (*zap.Logger, error) {
	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return nil, err
	}

	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(fileEncoderConfig()),
		zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    maxSizeMB,
			MaxBackups: maxBackups,
			MaxAge:     maxAgeDays,
			Compress:   false,
		}),
		cfg.Level,
	)

	core := fileCore
	if cfg.ConsoleEnabled {
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig()),
			zapcore.AddSync(os.Stderr),
			cfg.Level,
		)
		core = zapcore.NewTee(fileCore, consoleCore)
	}

	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}

func InstallGlobal(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}

	globalMu.Lock()
	globalLogger = logger
	globalMu.Unlock()
	zap.ReplaceGlobals(logger)
}

func Sync() error {
	globalMu.RLock()
	logger := globalLogger
	globalMu.RUnlock()
	if logger == nil {
		return nil
	}

	err := logger.Sync()
	if shouldIgnoreSyncError(err) {
		return nil
	}
	return err
}

func fileEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	cfg.TimeKey = "ts"
	cfg.MessageKey = "msg"
	cfg.LevelKey = "level"
	cfg.CallerKey = "caller"
	return cfg
}

func consoleEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	return cfg
}

func shouldIgnoreSyncError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrInvalid) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid argument") ||
		strings.Contains(message, "bad file descriptor") ||
		strings.Contains(message, "file already closed") ||
		strings.Contains(message, "sync /dev/stdout") ||
		strings.Contains(message, "sync /dev/stderr")
}
