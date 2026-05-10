package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
)

func TestLoadStateInitializesBootstrapStateFields(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC()
	state, err := LoadState()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	after := time.Now().UTC()

	if state.AppVersion != "0.0.1" {
		t.Fatalf("expected app version 0.0.1, got %q", state.AppVersion)
	}
	if state.DebugMode {
		t.Fatal("expected debug mode to default to false")
	}
	if state.Cwd == "" {
		t.Fatal("expected cwd to be initialized")
	}
	expectedProjectRoot, err := filepath.Abs(state.Cwd)
	if err != nil {
		t.Fatalf("resolve expected project root: %v", err)
	}
	if state.ProjectRoot != expectedProjectRoot {
		t.Fatalf("expected project root %q, got %q", expectedProjectRoot, state.ProjectRoot)
	}
	if state.StartTime == "" {
		t.Fatal("expected start time to be initialized")
	}
	startTime, err := time.Parse(time.RFC3339Nano, state.StartTime)
	if err != nil {
		t.Fatalf("parse start time: %v", err)
	}
	if startTime.Before(before) || startTime.After(after) {
		t.Fatalf("expected start time between %s and %s, got %s", before, after, startTime)
	}
	if state.SessionID == "" {
		t.Fatal("expected session id to be initialized")
	}
	sessionID, err := ksuid.Parse(state.SessionID)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	if sessionTime := sessionID.Time().UTC(); sessionTime.Before(before.Add(-time.Second)) || sessionTime.After(after.Add(time.Second)) {
		t.Fatalf("expected session id time between %s and %s, got %s", before.Add(-time.Second), after.Add(time.Second), sessionTime)
	}
}

func TestProviderConfigFromAppConfigRequiresAPIKeyAndModel(t *testing.T) {
	t.Parallel()

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
	if State == nil {
		t.Fatal("expected global state to be initialized")
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
	if err := runtime.GracefulShutdown(context.Background()); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}
