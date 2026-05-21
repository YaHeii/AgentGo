package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigFileReturnsEmptyWhenMissing(t *testing.T) {
	cfg, err := LoadConfigFile(t.TempDir())

	require.NoError(t, err)
	require.Empty(t, cfg.Servers)
}

func TestLoadConfigFileParsesServers(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
		"servers": [
			{
				"name": "fs",
				"kind": "stdio",
				"command": "mcp-server",
				"args": ["--root", "."],
				"env": ["MCP_ENV=test"]
			},
			{
				"name": "remote",
				"kind": "streamable_http",
				"url": "http://127.0.0.1:8080/mcp",
				"headers": {
					"Authorization": "Bearer token"
				}
			}
		]
	}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFileName), data, 0o600))

	cfg, err := LoadConfigFile(dir)

	require.NoError(t, err)
	require.Len(t, cfg.Servers, 2)
	require.Equal(t, "fs", cfg.Servers[0].Name)
	require.Equal(t, TransportStdio, cfg.Servers[0].Kind)
	require.Equal(t, "mcp-server", cfg.Servers[0].Command)
	require.Equal(t, []string{"--root", "."}, cfg.Servers[0].Args)
	require.Equal(t, []string{"MCP_ENV=test"}, cfg.Servers[0].Env)
	require.Equal(t, "remote", cfg.Servers[1].Name)
	require.Equal(t, TransportStreamableHTTP, cfg.Servers[1].Kind)
	require.Equal(t, "http://127.0.0.1:8080/mcp", cfg.Servers[1].URL)
	require.Equal(t, "Bearer token", cfg.Servers[1].Headers["Authorization"])
}

func TestLoadConfigFileValidatesServers(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"servers":[{"name":"fs","kind":"stdio"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFileName), data, 0o600))

	cfg, err := LoadConfigFile(dir)

	require.Empty(t, cfg.Servers)
	require.ErrorContains(t, err, "stdio command is required")
}
