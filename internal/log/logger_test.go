package log

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLoggerWritesConsoleAndFileInDevelopment(t *testing.T) {
	restore, output := captureStderr(t)
	defer restore()

	dir := t.TempDir()
	resolved, err := ResolveConfig(Config{
		Environment: "development",
		LogDir:      dir,
		LogLevel:    "info",
		ProjectRoot: dir,
	})
	require.NoError(t, err)

	logger, err := NewLogger(resolved)
	require.NoError(t, err)
	InstallGlobal(logger)
	t.Cleanup(func() {
		require.NoError(t, Sync())
	})

	logger.Info("development-log-message")
	require.NoError(t, Sync())

	stderrOutput := output()
	require.Contains(t, stderrOutput, "development-log-message")

	fileData, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	require.NoError(t, err)
	require.Contains(t, string(fileData), "development-log-message")
}

func TestNewLoggerWritesFileOnlyInRelease(t *testing.T) {
	restore, output := captureStderr(t)
	defer restore()

	dir := t.TempDir()
	resolved, err := ResolveConfig(Config{
		Environment: "release",
		LogDir:      dir,
		LogLevel:    "info",
		ProjectRoot: dir,
	})
	require.NoError(t, err)

	logger, err := NewLogger(resolved)
	require.NoError(t, err)
	InstallGlobal(logger)
	t.Cleanup(func() {
		require.NoError(t, Sync())
	})

	logger.Info("release-log-message")
	require.NoError(t, Sync())

	require.Empty(t, strings.TrimSpace(output()))

	fileData, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	require.NoError(t, err)
	require.Contains(t, string(fileData), "release-log-message")
}

func captureStderr(t *testing.T) (func(), func() string) {
	t.Helper()

	original := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer

	restore := func() {
		_ = writer.Close()
		os.Stderr = original
	}
	output := func() string {
		_ = writer.Close()
		data, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		return string(data)
	}
	return restore, output
}
