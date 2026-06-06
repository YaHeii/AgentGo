# Slash Command Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `/historySession`, `/permission`, and `/newSession` execution in the terminal UI while keeping `/compact` explicitly unimplemented.

**Architecture:** Reuse the existing slash menu list as a temporary selection surface. `SlashMenu` owns list mode and selected item extraction; `UI` owns command routing, app facade calls, lifecycle permission mutation, and status updates.

**Tech Stack:** Go, Bubble Tea v2, Bubbles list, existing `app.APPService`, `lifecycle.GlobalState`, and `internal/ui/model` tests.

---

## File Structure

- Modify `internal/ui/model/slash_menu.go`: add menu modes and typed list items for commands, sessions, and permissions.
- Modify `internal/ui/model/ui.go`: extend `appService`, route selected slash menu items, and add async command messages/cmds.
- Modify `internal/ui/model/ui_test.go`: add TDD coverage for history session selection, permission selection, new session, and compact.

---

### Task 1: History Session List And Selection

**Files:**
- Modify: `internal/ui/model/ui_test.go`
- Modify: `internal/ui/model/slash_menu.go`
- Modify: `internal/ui/model/ui.go`

- [ ] **Step 1: Write the failing test**

Add a test in `internal/ui/model/ui_test.go` that opens `/historySession`, runs the returned load command, moves selection down, and presses Enter. The test should assert:

```go
func TestHistorySessionCommandListsSessionsAndSwitchesSelection(t *testing.T) {
	svc := newStubAppService()
	svc.sessions = []sessioncontract.Session{
		{ID: "session-1", Title: "First"},
		{ID: "session-2", Title: "Second"},
	}
	ui := New(svc)
	ui.width = 80
	ui.height = 24

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	ui = updated.(*UI)
	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected sessions load command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if !ui.slashMenu.IsOpen() || !strings.Contains(ui.slashMenu.View(), "Second") {
		t.Fatalf("expected session list view, got %q", ui.slashMenu.View())
	}

	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	ui = updated.(*UI)
	updated, cmd = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected switch session command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if svc.lastSwitchSessionID != "session-2" {
		t.Fatalf("expected switched session-2, got %q", svc.lastSwitchSessionID)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed after switch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run TestHistorySessionCommandListsSessionsAndSwitchesSelection -count=1
```

Expected: FAIL because `appService` lacks session methods and slash execution still returns `not implemented`.

- [ ] **Step 3: Write minimal implementation**

Implement only what is needed for session list selection:

```go
type slashMenuMode string

const (
	slashMenuModeCommands slashMenuMode = "commands"
	slashMenuModeSessions slashMenuMode = "sessions"
)
```

Add session items to `slash_menu.go`, add `OpenSessions`, `SelectedSessionID`, and make `Close` restore command mode. Extend `appService` in `ui.go` with `ListSessions` and `SwitchSession`. Add `sessionsLoadedMsg`, `switchSessionDoneMsg`, `listSessionsCmd`, and `switchSessionCmd`. Route `/historySession` to `listSessionsCmd`, and route Enter in session mode to `switchSessionCmd`.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run TestHistorySessionCommandListsSessionsAndSwitchesSelection -count=1
```

Expected: PASS.

---

### Task 2: Permission List And Runtime State Update

**Files:**
- Modify: `internal/ui/model/ui_test.go`
- Modify: `internal/ui/model/slash_menu.go`
- Modify: `internal/ui/model/ui.go`

- [ ] **Step 1: Write the failing test**

Add a test that opens `/permission`, selects `attention`, and asserts lifecycle state changes:

```go
func TestPermissionCommandUpdatesLifecyclePermissionLevel(t *testing.T) {
	t.Cleanup(func() { lifecycle.State = nil })
	lifecycle.State = &lifecycle.GlobalState{PermissionLevel: lifecycle.SafeLevel}

	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.input.Append("/per")
	ui.syncSlashMenuFromInput()
	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if !ui.slashMenu.IsOpen() || !strings.Contains(ui.slashMenu.View(), "attention") {
		t.Fatalf("expected permission list view, got %q", ui.slashMenu.View())
	}

	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	ui = updated.(*UI)
	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if lifecycle.State.PermissionLevel != lifecycle.AttentionLevel {
		t.Fatalf("expected attention permission, got %v", lifecycle.State.PermissionLevel)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run TestPermissionCommandUpdatesLifecyclePermissionLevel -count=1
```

Expected: FAIL because permission mode does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add `slashMenuModePermissions`, a `permissionItem`, `OpenPermissions`, and `SelectedPermissionLevel`. In `ui.go`, route `/permission` to `OpenPermissions`; when Enter is pressed in permission mode, write `lifecycle.State.PermissionLevel`, close the menu, clear input, recompute layout, and set a transient status such as `permission: attention`.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run TestPermissionCommandUpdatesLifecyclePermissionLevel -count=1
```

Expected: PASS.

---

### Task 3: New Session And Compact Commands

**Files:**
- Modify: `internal/ui/model/ui_test.go`
- Modify: `internal/ui/model/ui.go`

- [ ] **Step 1: Write failing tests**

Add tests:

```go
func TestNewSessionCommandStartsNewSession(t *testing.T) {
	svc := newStubAppService()
	ui := New(svc)
	ui.width = 80
	ui.height = 24
	ui.input.Append("/new")
	ui.syncSlashMenuFromInput()
	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected new session command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if svc.lastNewSessionTitle != "New Session" {
		t.Fatalf("expected New Session title, got %q", svc.lastNewSessionTitle)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed")
	}
}

func TestCompactCommandRemainsExplicitlyUnimplemented(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.input.Append("/compact")
	ui.syncSlashMenuFromInput()
	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd != nil {
		t.Fatal("expected no command for compact")
	}
	if !strings.Contains(ui.header.TransientStatus(), "not implemented: /compact") {
		t.Fatalf("expected compact not implemented status, got %q", ui.header.TransientStatus())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run 'Test(NewSessionCommandStartsNewSession|CompactCommandRemainsExplicitlyUnimplemented)' -count=1
```

Expected: FAIL because `/newSession` does not call app service yet and existing compact routing may not match the exact behavior.

- [ ] **Step 3: Write minimal implementation**

Extend `appService` with `StartNewSession`. Add `startNewSessionDoneMsg` and `startNewSessionCmd`. Route `/newSession` to `startNewSessionCmd(u.app, "New Session")`; route `/compact` to the existing status-only behavior.

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -run 'Test(NewSessionCommandStartsNewSession|CompactCommandRemainsExplicitlyUnimplemented)' -count=1
```

Expected: PASS.

---

### Task 4: Integration Verification

**Files:**
- Modify: `internal/ui/model/ui_test.go`

- [ ] **Step 1: Update test stub**

Ensure `stubAppService` implements the expanded `appService` interface:

```go
sessions            []sessioncontract.Session
lastSwitchSessionID string
lastNewSessionTitle string
listSessionsErr     error
switchSessionErr    error
startNewSessionErr  error
```

Add methods:

```go
func (s *stubAppService) ListSessions(context.Context) ([]sessioncontract.Session, error)
func (s *stubAppService) SwitchSession(context.Context, string) error
func (s *stubAppService) StartNewSession(context.Context, string) error
```

- [ ] **Step 2: Run UI package tests**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./internal/ui/model -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full repository tests**

Run:

```bash
GOCACHE=/tmp/go-build /usr/local/go/bin/go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 4: Inspect diff**

Run:

```bash
git diff -- internal/ui/model/slash_menu.go internal/ui/model/ui.go internal/ui/model/ui_test.go
```

Expected: diff only touches slash command execution and its tests.
