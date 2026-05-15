package lifecycle

import (
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

func TestPermissionLevelOrderingMatchesToolSecurityLevel(t *testing.T) {
	tests := []struct {
		name       string
		permission PermissionLevel
		security   toolcontract.SecurityLevel
	}{
		{name: "safe", permission: SafeLevel, security: toolcontract.SafeLevel},
		{name: "attention", permission: AttentionLevel, security: toolcontract.AttentionLevel},
		{name: "danger", permission: DangerLevel, security: toolcontract.DangerLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolcontract.SecurityLevel(tt.permission); got != tt.security {
				t.Fatalf("expected %v, got %v", tt.security, got)
			}
		})
	}
}

func TestStateAllowsDirectWriteByDesign(t *testing.T) {
	State = &GlobalState{}
	t.Cleanup(func() {
		State = nil
	})

	State.PermissionLevel = AttentionLevel
	if State.PermissionLevel != AttentionLevel {
		t.Fatalf("expected permission level %v, got %v", AttentionLevel, State.PermissionLevel)
	}

	State.PermissionLevel = SafeLevel
	if State.PermissionLevel != SafeLevel {
		t.Fatalf("expected permission level %v, got %v", SafeLevel, State.PermissionLevel)
	}
}
