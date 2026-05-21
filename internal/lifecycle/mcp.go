package lifecycle

import (
	"context"

	"github.com/YaHeii/agentGo/internal/tool/mcp"
)

type mcpClient interface {
	mcp.Client
}

var mcpClientFactory = newMCPClient

func newMCPClient(cfg mcp.Config) (mcpClient, error) {
	return mcp.NewClient(cfg)
}

func (s *Supervisor) initializeMCPTools(ctx context.Context, configDir string) error {
	if configDir == "" {
		configDir = "."
	}

	fileConfig, err := mcp.LoadConfigFile(configDir)
	if err != nil {
		return err
	}

	for _, server := range fileConfig.Servers {
		client, err := mcpClientFactory(server)
		if err != nil {
			return err
		}
		if err := client.Start(ctx); err != nil {
			_ = client.Close()
			return err
		}

		metas, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close()
			return err
		}
		s.mcpClients = append(s.mcpClients, client)
		for _, meta := range metas {
			s.tools = append(s.tools, mcp.NewRemoteTool(server.Name, client, meta))
		}
	}
	return nil
}
