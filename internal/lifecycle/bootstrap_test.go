package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/YaHeii/agentGo/internal/tool/mcp"
	"github.com/segmentio/ksuid"
)

func TestSupervisorInitializePopulatesBootstrapState(t *testing.T) {
	State = &GlobalState{}
	CurrentSupervisor = nil
	t.Cleanup(func() {
		State = nil
		CurrentSupervisor = nil
	})
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Model:         "test-model",
		ContextWindow: 400000,
	})

	err := supervisor.Initialize(context.Background())
	if err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	if State.AppVersion != "0.0.1" {
		t.Fatalf("expected app version 0.0.1, got %q", State.AppVersion)
	}
	if State.StartTime == "" {
		t.Fatal("expected start time to be initialized")
	}
	if State.SessionID == "" {
		t.Fatal("expected session id to be initialized")
	}
	if _, err := ksuid.Parse(State.SessionID); err != nil {
		t.Fatalf("expected ksuid session id, got %v", err)
	}
	if State.Cwd == "" {
		t.Fatal("expected cwd to be initialized")
	}
	if State.ProjectRoot == "" {
		t.Fatal("expected project root to be initialized")
	}
	if len(State.InitialEnv) == 0 {
		t.Fatal("expected initial env to be populated")
	}
	if State.ModelLimit != 400000 {
		t.Fatalf("expected model limit 400000, got %d", State.ModelLimit)
	}
	if State.MaxTurn != 0 {
		t.Fatalf("expected default max turn 0, got %d", State.MaxTurn)
	}
	if len(State.KnownTools) == 0 {
		t.Fatal("expected at least one known tool")
	}
	tools, ok := any(State.KnownTools).([]toolcontract.Metadata)
	if !ok {
		t.Fatalf("expected known tools to use tool contract metadata, got %T", State.KnownTools)
	}
	if tools[0].Name != "grep" {
		t.Fatalf("expected grep tool to be discovered, got %+v", tools[0])
	}
}

func TestSupervisorInitializeRegistersMCPToolsFromConfigFile(t *testing.T) {
	State = &GlobalState{}
	CurrentSupervisor = nil
	t.Cleanup(func() {
		State = nil
		CurrentSupervisor = nil
		mcpClientFactory = newMCPClient
	})

	configDir := t.TempDir()
	mcpConfig := `{
		"servers": [
			{
				"name": "fs",
				"kind": "stdio",
				"command": "mcp-server"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(configDir, "MCP.json"), []byte(mcpConfig), 0o600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}

	mcpClientFactory = func(mcp.Config) (mcpClient, error) {
		return &stubMCPClient{
			tools: []toolcontract.Metadata{{
				Name:          "read_file",
				Description:   "Read a file",
				Parameters:    []byte(`{"type":"object"}`),
				Enabled:       true,
				SecurityLevel: toolcontract.AttentionLevel,
			}},
		}, nil
	}

	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Model:         "test-model",
		ContextWindow: 400000,
		ConfigDir:     configDir,
	})

	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	var found bool
	for _, meta := range State.KnownTools {
		if meta.Name == "fs__read_file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MCP tool in known tools, got %+v", State.KnownTools)
	}
}

func TestLoadConfigStoresConfigDir(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.env")
	config := "API_KEY=test-key\nMODEL=test-model\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ConfigDir != configDir {
		t.Fatalf("expected config dir %q, got %q", configDir, cfg.ConfigDir)
	}
}

func TestSupervisorInitializeCopiesConfigIntoState(t *testing.T) {
	State = &GlobalState{}
	CurrentSupervisor = nil
	t.Cleanup(func() {
		State = nil
		CurrentSupervisor = nil
	})
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Model:         "test-model",
		ContextWindow: 12345,
		MaxTurn:       9,
	})

	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	if State.ModelLimit != 12345 {
		t.Fatalf("expected model limit 12345, got %d", State.ModelLimit)
	}
	if State.MaxTurn != 9 {
		t.Fatalf("expected max turn 9, got %d", State.MaxTurn)
	}
}

func TestRuntimeCloseCallsCloser(t *testing.T) {
	called := false
	runtime := Runtime{
		closeFn: func(context.Context) error {
			called = true
			return nil
		},
	}

	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if !called {
		t.Fatal("expected close function to be called")
	}
}

func TestRuntimeCloseWithoutCloserIsNoop(t *testing.T) {
	runtime := Runtime{}
	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func TestBootstrapInitializesGlobalStateAndSupervisor(t *testing.T) {
	State = nil
	CurrentSupervisor = nil
	t.Cleanup(func() {
		State = nil
		CurrentSupervisor = nil
	})

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.env")
	config := "API_KEY=test-key\nMODEL=test-model\nCONTEXT_WINDOW=400000\nMAX_TOKENS=128000\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	dispatcher := app.NewDispatcher(16)
	State = &GlobalState{}
	supervisor := NewSupervisor(dispatcher, cfg)
	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}
	CurrentSupervisor = supervisor

	if State == nil {
		t.Fatal("expected global state to be initialized")
	}
	if CurrentSupervisor == nil {
		t.Fatal("expected current supervisor to be initialized")
	}

	if State.Cwd == "" {
		t.Fatal("expected current cwd to be initialized")
	}
	if State.ProjectRoot == "" {
		t.Fatal("expected project root to be initialized")
	}
	if _, err := ksuid.Parse(State.SessionID); err != nil {
		t.Fatalf("expected bootstrap session id to be a ksuid: %v", err)
	}
	if len(State.KnownTools) == 0 {
		t.Fatal("expected known tools to be initialized")
	}

	payload := BootstrapDoneEvent{
		Time:      State.StartTime,
		SessionID: State.SessionID,
	}
	if payload.Time == "" {
		t.Fatal("expected bootstrap event time")
	}
	if payload.SessionID == "" {
		t.Fatal("expected bootstrap event session id")
	}
}
