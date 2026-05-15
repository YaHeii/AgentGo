package lifecycle

import (
	"encoding/json"
	"testing"

	"github.com/YaHeii/agentGo/internal/tool"
)

func TestPermissionLevelOrderingMatchesToolSecurityLevel(t *testing.T) {
	tests := []struct {
		name       string
		permission PermissionLevel
		security   tool.SecurityLevel
	}{
		{name: "safe", permission: SafeLevel, security: tool.SafeLevel},
		{name: "attention", permission: AttentionLevel, security: tool.AttentionLevel},
		{name: "danger", permission: DangerLevel, security: tool.DangerLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tool.SecurityLevel(tt.permission); got != tt.security {
				t.Fatalf("expected %v, got %v", tt.security, got)
			}
		})
	}
}

func TestSetPermissionLevelInitializesAndUpdatesState(t *testing.T) {
	resetGlobalStateForTest()
	SetPermissionLevel(AttentionLevel)

	if got := GetState().PermissionLevel; got != AttentionLevel {
		t.Fatalf("expected permission level %v, got %v", AttentionLevel, got)
	}

	SetPermissionLevel(SafeLevel)
	if got := GetState().PermissionLevel; got != SafeLevel {
		t.Fatalf("expected permission level %v, got %v", SafeLevel, got)
	}
}

func TestGetStateDeepCopiesMutableFields(t *testing.T) {
	resetGlobalStateForTest()
	State.initialize(GlobalState{
		AppVersion:  "0.0.1",
		StartTime:   "now",
		Cwd:         "/tmp/work",
		ProjectRoot: "/tmp/work",
		SessionID:   "session",
		InitialEnv: map[string]string{
			"PWD": "/tmp/work",
		},
		KnownTools: []ToolSnapshot{
			{
				Name:        "grep",
				Description: "search",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
	})

	snapshot := GetState()
	snapshot.InitialEnv["PWD"] = "/mutated"
	snapshot.KnownTools[0].Name = "changed"

	after := GetState()
	if after.InitialEnv["PWD"] != "/tmp/work" {
		t.Fatalf("expected env deep copy, got %q", after.InitialEnv["PWD"])
	}
	if after.KnownTools[0].Name != "grep" {
		t.Fatalf("expected tool snapshot deep copy, got %q", after.KnownTools[0].Name)
	}
	if string(after.KnownTools[0].Parameters) != `{"type":"object"}` {
		t.Fatalf("expected tool parameters to be preserved, got %s", string(after.KnownTools[0].Parameters))
	}
}
