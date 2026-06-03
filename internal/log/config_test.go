package log

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveConfigDefaultsAndDevelopmentMode(t *testing.T) {
	resolved, err := ResolveConfig(Config{
		Environment: "development",
		ProjectRoot: "/workspace/project",
	})
	require.NoError(t, err)
	require.Equal(t, "development", resolved.Environment)
	require.Equal(t, "info", resolved.LogLevel)
	require.Equal(t, filepath.Join("/workspace/project", "logs"), resolved.LogDir)
	require.Equal(t, filepath.Join("/workspace/project", "logs", DefaultFileName), resolved.FilePath)
	require.True(t, resolved.ConsoleEnabled)
}

func TestResolveConfigReleaseDisablesConsole(t *testing.T) {
	resolved, err := ResolveConfig(Config{
		Environment: "release",
		LogDir:      "/tmp/agentgo-logs",
		LogLevel:    "debug",
		ProjectRoot: "/workspace/project",
	})
	require.NoError(t, err)
	require.Equal(t, "release", resolved.Environment)
	require.Equal(t, "debug", resolved.LogLevel)
	require.False(t, resolved.ConsoleEnabled)
}

func TestResolveConfigRejectsInvalidEnvironment(t *testing.T) {
	_, err := ResolveConfig(Config{
		Environment: "staging",
		ProjectRoot: "/workspace/project",
	})
	require.Error(t, err)
}

func TestResolveConfigRejectsInvalidLogLevel(t *testing.T) {
	_, err := ResolveConfig(Config{
		Environment: "development",
		LogLevel:    "verbose",
		ProjectRoot: "/workspace/project",
	})
	require.Error(t, err)
}
