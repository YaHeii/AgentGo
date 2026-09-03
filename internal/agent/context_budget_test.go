package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/YaHeii/agentGo/internal/lifecycle"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestSelectHistoryUsesTokenBudgetInsteadOfByteLength(t *testing.T) {
	originalCounter := countTokens
	countTokens = fakeTokenCounter
	t.Cleanup(func() { countTokens = originalCounter })

	history := []messagecontract.Message{
		{ID: "old", Kind: messagecontract.KindUser, Parts: []messagecontract.Part{
			{Type: messagecontract.PartTypeText, Text: strings.Repeat("中", 100)},
		}},
		{ID: "new", Kind: messagecontract.KindUser, Parts: []messagecontract.Part{
			{Type: messagecontract.PartTypeText, Text: "latest"},
		}},
	}

	selected, err := selectHistoryByTokenBudget("gpt-4o-mini", history, 20)
	require.NoError(t, err)
	require.Equal(t, []string{"new"}, messageIDs(selected))
}

func TestSelectHistoryKeepsToolCallAndResultTogether(t *testing.T) {
	originalCounter := countTokens
	countTokens = fakeTokenCounter
	t.Cleanup(func() { countTokens = originalCounter })

	history := []messagecontract.Message{
		{
			ID: "old", Kind: messagecontract.KindUser,
			Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: strings.Repeat("old ", 20)}},
		},
		assistantToolCallMessage("assistant-tool", "call-1"),
		toolResultMessage("tool-result", "call-1", "result"),
		userMessage("latest"),
	}

	selected, err := selectHistoryByTokenBudget("gpt-4o-mini", history, 70)
	require.NoError(t, err)
	require.Equal(t, []string{"assistant-tool", "tool-result", "latest"}, messageIDs(selected))
	require.NotContains(t, messageIDs(selected), "tool-result-only")
}

func TestBuildContextBudgetIncludesSystemToolsAndOutputReservation(t *testing.T) {
	originalCounter := countTokens
	countTokens = fakeTokenCounter
	t.Cleanup(func() { countTokens = originalCounter })

	budget, err := calculateContextBudget(
		"gpt-4o-mini",
		1000,
		100,
		"system "+strings.Repeat("prompt ", 20),
		[]toolcontract.Metadata{{
			Name:        "grep",
			Description: "search",
			Parameters:  []byte(`{"type":"object","properties":{"pattern":{"type":"string"}}}`),
		}},
	)
	require.NoError(t, err)
	require.Less(t, budget.HistoryBudget, 900)
	require.Equal(t, 100, budget.OutputTokens)
	require.Greater(t, budget.FixedTokens, 0)
}

func TestContextBudgetUsesNinetyPercentCompactionThreshold(t *testing.T) {
	originalCounter := countTokens
	countTokens = fakeTokenCounter
	t.Cleanup(func() { countTokens = originalCounter })

	budget, err := calculateContextBudget("gpt-4o-mini", 1000, 0, "", nil)
	require.NoError(t, err)
	require.Equal(t, 900, budget.CompactionThreshold)
	require.True(t, budget.ShouldCompact(900))
	require.False(t, budget.ShouldCompact(899))
}

func TestBuildCompactSummaryMessageMarksMessageAsCompactSummary(t *testing.T) {
	summary := buildCompactSummaryMessage("session-1", []messagecontract.Message{
		userMessage("old"),
		userMessage("older"),
	})

	require.Equal(t, "session-1", summary.SessionID)
	require.Equal(t, messagecontract.KindSystem, summary.Kind)
	require.True(t, summary.IsCompactSummary)
	require.Len(t, summary.Parts, 1)
	require.Equal(t, messagecontract.PartTypeSummary, summary.Parts[0].Type)
	require.Contains(t, summary.Parts[0].Text, "old")
}

func TestRenderCompactionPromptUsesTemplateSections(t *testing.T) {
	loop := NewQueryLoop(nil, nil, nil)
	prompt, err := loop.renderCompactionPrompt(
		"修复上下文压缩",
		"已有决策：使用 token 预算",
		[]messagecontract.Message{userMessage("recent task")},
		[]messagecontract.Message{toolResultMessage("tool-1", "call-1", "保留 /tmp/result.txt")},
		128,
	)
	require.NoError(t, err)
	require.Contains(t, prompt, "## Current Task")
	require.Contains(t, prompt, "修复上下文压缩")
	require.Contains(t, prompt, "已有决策：使用 token 预算")
	require.Contains(t, prompt, "保留 /tmp/result.txt")
	require.Contains(t, prompt, "## Output Requirements")
	require.Contains(t, prompt, "128 tokens")
}

func TestPrepareHistoryPersistsCompactSummaryAtNinetyPercent(t *testing.T) {
	originalCounter := countTokens
	originalState := lifecycle.State
	countTokens = fakeTokenCounter
	lifecycle.State = &lifecycle.GlobalState{Model: "gpt-4o-mini", ModelLimit: 1000}
	t.Cleanup(func() {
		countTokens = originalCounter
		lifecycle.State = originalState
	})

	app := &contextBudgetApp{}
	loop := NewQueryLoop(app, nil, nil)
	history := []messagecontract.Message{
		{
			ID: "old", SessionID: "session-1", Kind: messagecontract.KindUser,
			Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: strings.Repeat("o", 700)}},
		},
		{
			ID: "latest", SessionID: "session-1", Kind: messagecontract.KindUser,
			Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: strings.Repeat("n", 250)}},
		},
	}

	prepared, err := loop.prepareHistory(context.Background(), "session-1", history, "")
	require.NoError(t, err)
	require.Len(t, app.created, 1)
	require.True(t, app.created[0].IsCompactSummary)
	require.Equal(t, messagecontract.PartTypeSummary, app.created[0].Parts[0].Type)
	require.LessOrEqual(t, len([]rune(app.created[0].Parts[0].Text)), 100)
	require.Contains(t, app.created[0].Parts[0].Text, strings.Repeat("n", 20))
	require.True(t, prepared[0].IsCompactSummary)
	require.Equal(t, "latest", prepared[len(prepared)-1].ID)
}

func TestPrepareHistoryRendersCompactionTemplateBeforeProviderCall(t *testing.T) {
	originalCounter := countTokens
	originalState := lifecycle.State
	countTokens = fakeTokenCounter
	lifecycle.State = &lifecycle.GlobalState{Model: "gpt-4o-mini", ModelLimit: 1000}
	t.Cleanup(func() {
		countTokens = originalCounter
		lifecycle.State = originalState
	})

	app := &contextBudgetApp{}
	provider := &compactionProvider{}
	loop := NewQueryLoop(app, provider, nil)
	history := []messagecontract.Message{
		{
			ID: "old", SessionID: "session-1", Kind: messagecontract.KindUser,
			Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: strings.Repeat("o", 700)}},
		},
		{
			ID: "latest", SessionID: "session-1", Kind: messagecontract.KindUser,
			Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: strings.Repeat("n", 250)}},
		},
	}

	_, err := loop.prepareHistory(context.Background(), "session-1", history, "当前任务：修复压缩")
	require.NoError(t, err)
	require.Len(t, provider.requests, 1)
	require.Contains(t, provider.requests[0].Messages[0].Parts[0].Text, "当前任务：修复压缩")
	require.Contains(t, provider.requests[0].Messages[0].Parts[0].Text, "Messages To Compress")
	require.Contains(t, app.created[0].Parts[0].Text, "provider summary")
	require.True(t, app.created[0].IsCompactSummary)
}

func fakeTokenCounter(_ string, text string) (int, error) {
	return len([]rune(text)), nil
}

func messageIDs(messages []messagecontract.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}
	return ids
}

func userMessage(id string) messagecontract.Message {
	return messagecontract.Message{
		ID: id, Kind: messagecontract.KindUser,
		Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: id}},
	}
}

func assistantToolCallMessage(id, callID string) messagecontract.Message {
	return messagecontract.Message{
		ID: id, Kind: messagecontract.KindAssistant,
		Parts: []messagecontract.Part{{Type: messagecontract.PartTypeToolCall, ToolCall: &messagecontract.ToolCallPart{
			ID: callID, Name: "grep", Input: `{"pattern":"go"}`,
		}}},
	}
}

func toolResultMessage(id, callID, content string) messagecontract.Message {
	return messagecontract.Message{
		ID: id, Kind: messagecontract.KindSystem,
		Parts: []messagecontract.Part{{Type: messagecontract.PartTypeToolResult, ToolResult: &messagecontract.ToolResultPart{
			ToolCallID: callID, Content: content,
		}}},
	}
}

type contextBudgetApp struct {
	created []messagecontract.CreateMessageParams
}

type compactionProvider struct {
	requests []providercontract.Request
}

func (p *compactionProvider) RunTurn(_ context.Context, req providercontract.Request) (providercontract.TurnResult, error) {
	p.requests = append(p.requests, req)
	return providercontract.TurnResult{Text: "# Task\n\n## Goal\n- provider summary\n\n## Verification\n- pending\n\n## Open Items\n- none"}, nil
}

func (a *contextBudgetApp) ListHistory(context.Context, string) ([]messagecontract.Message, error) {
	return nil, nil
}

func (a *contextBudgetApp) CreateMessage(_ context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error) {
	a.created = append(a.created, params)
	return messagecontract.Message{
		ID:               "summary-1",
		SessionID:        params.SessionID,
		Kind:             params.Kind,
		IsCompactSummary: params.IsCompactSummary,
		Parts:            params.Parts,
	}, nil
}

func (a *contextBudgetApp) ListTools(context.Context, toolcontract.SecurityLevel) []toolcontract.Metadata {
	return nil
}

func (a *contextBudgetApp) CallTools(context.Context, toolcontract.BatchRequest) ([]toolcontract.ToolResult, error) {
	return nil, nil
}
