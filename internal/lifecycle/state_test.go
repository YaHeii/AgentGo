package lifecycle

import "testing"

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
			{Name: "grep", Description: "search"},
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
}
