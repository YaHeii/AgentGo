package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YaHeii/agentGo/internal/utils"
)

func TestGlobalStateExposesConversationSeedFields(t *testing.T) {
	t.Parallel()

	state := &GlobalState{}
	state.SetCurrentCWD("/tmp/project")
	state.SetProjectRoot("/tmp/project")
	state.SetSessionSeed("session-1", "parent-1")

	if got := state.CurrentCWD(); got != "/tmp/project" {
		t.Fatalf("expected current cwd /tmp/project, got %q", got)
	}
	if got := state.ProjectRoot(); got != "/tmp/project" {
		t.Fatalf("expected project root /tmp/project, got %q", got)
	}
	if got := state.SessionID(); got != "session-1" {
		t.Fatalf("expected session id session-1, got %q", got)
	}
	if got := state.ParentSessionID(); got != "parent-1" {
		t.Fatalf("expected parent session id parent-1, got %q", got)
	}
}

func TestProviderConfigFromAppConfigRequiresAPIKeyAndModel(t *testing.T) {
	t.Parallel()

	_, err := ProviderConfigFromAppConfig(utils.Config{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("expected API_KEY error, got %v", err)
	}

	_, err = ProviderConfigFromAppConfig(utils.Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing MODEL error")
	}
	if !strings.Contains(err.Error(), "MODEL") {
		t.Fatalf("expected MODEL error, got %v", err)
	}
}

func TestRuntimeCloseCallsCloser(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	runtime := Runtime{}
	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func TestBootstrapReturnsRuntimeWithAppAndCloser(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "app.env")
	config := "API_KEY=test-key\nMODEL=test-model\n"
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
	if runtime.Bus == nil {
		t.Fatal("expected unified app event bus")
	}
	if Global == nil {
		t.Fatal("expected global state to be initialized")
	}
	if Global.CurrentCWD() == "" {
		t.Fatal("expected current cwd to be initialized")
	}
	if Global.ProjectRoot() == "" {
		t.Fatal("expected project root to be initialized")
	}
	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
