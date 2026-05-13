package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/segmentio/ksuid"
)

func TestSupervisorInitializePopulatesBootstrapState(t *testing.T) {
	resetGlobalStateForTest()
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Model:         "test-model",
		ContextWindow: 400000,
	})

	err := supervisor.Initialize(context.Background())
	if err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	snapshot := GetState()
	if snapshot.AppVersion != "0.0.1" {
		t.Fatalf("expected app version 0.0.1, got %q", snapshot.AppVersion)
	}
	if snapshot.StartTime == "" {
		t.Fatal("expected start time to be initialized")
	}
	if snapshot.SessionID == "" {
		t.Fatal("expected session id to be initialized")
	}
	if _, err := ksuid.Parse(snapshot.SessionID); err != nil {
		t.Fatalf("expected ksuid session id, got %v", err)
	}
	if snapshot.Cwd == "" {
		t.Fatal("expected cwd to be initialized")
	}
	if snapshot.ProjectRoot == "" {
		t.Fatal("expected project root to be initialized")
	}
	if len(snapshot.InitialEnv) == 0 {
		t.Fatal("expected initial env to be populated")
	}
	if snapshot.ModelLimit != 400000 {
		t.Fatalf("expected model limit 400000, got %d", snapshot.ModelLimit)
	}
	if len(snapshot.KnownTools) == 0 {
		t.Fatal("expected at least one known tool")
	}
	if snapshot.KnownTools[0].Name != "grep" {
		t.Fatalf("expected grep tool to be discovered, got %+v", snapshot.KnownTools[0])
	}
}

func TestProviderConfigFromAppConfigRequiresAPIKeyAndModel(t *testing.T) {
	_, err := ProviderConfigFromAppConfig(Config{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("expected API_KEY error, got %v", err)
	}

	_, err = ProviderConfigFromAppConfig(Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing MODEL error")
	}
	if !strings.Contains(err.Error(), "MODEL") {
		t.Fatalf("expected MODEL error, got %v", err)
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

func TestBootstrapReturnsRuntimeWithAppAndCloser(t *testing.T) {
	resetGlobalStateForTest()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.env")
	config := "API_KEY=test-key\nMODEL=test-model\nCONTEXT_WINDOW=400000\nMAX_TOKENS=128000\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	databasePath := filepath.Join(configDir, "agentgo.db")

	runtime, err := Bootstrap(context.Background(), configDir, databasePath)
	if err != nil {
		t.Fatalf("bootstrap runtime: %v", err)
	}
	if runtime.App == nil {
		t.Fatal("expected app service")
	}
	if State == nil {
		t.Fatal("expected global state to be initialized")
	}

	snapshot := GetState()
	if snapshot.Cwd == "" {
		t.Fatal("expected current cwd to be initialized")
	}
	if snapshot.ProjectRoot == "" {
		t.Fatal("expected project root to be initialized")
	}
	if _, err := ksuid.Parse(snapshot.SessionID); err != nil {
		t.Fatalf("expected bootstrap session id to be a ksuid: %v", err)
	}
	if len(snapshot.KnownTools) == 0 {
		t.Fatal("expected known tools to be initialized")
	}

	payload := BootstrapDoneEvent{
		Time:      snapshot.StartTime,
		SessionID: snapshot.SessionID,
	}
	if payload.Time == "" {
		t.Fatal("expected bootstrap event time")
	}
	if payload.SessionID == "" {
		t.Fatal("expected bootstrap event session id")
	}

	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
