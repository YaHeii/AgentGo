package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	
	// "github.com/stretchr/testify/require"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/utils"
)

func TestValidateProviderConfigRequiresAPIKeyAndModel(t *testing.T) {
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

func TestEnterSendsMessageAndAppendsAssistantReply(t *testing.T) {
	t.Parallel()

	llm := &stubLLM{reply: "Hello back"}
	m := NewChatModel(llm)
	m.input = "Hello"

	updated, cmd := m.Update(enterKey())
	next := updated.(chatModel)

	if !next.loading {
		t.Fatal("expected loading state after enter")
	}
	if next.input != "" {
		t.Fatalf("expected cleared input, got %q", next.input)
	}
	if len(next.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(next.messages))
	}
	if next.messages[0].Role != roleUser || next.messages[0].Content != "Hello" {
		t.Fatalf("unexpected user message: %+v", next.messages[0])
	}
	if cmd == nil {
		t.Fatal("expected async command")
	}

	msg := cmd()
	if len(llm.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(llm.calls))
	}
	if got := llm.calls[0]; len(got) != 1 || got[0].Role != roleUser || got[0].Content != "Hello" {
		t.Fatalf("unexpected provider history: %+v", got)
	}

	updated, _ = next.Update(msg)
	next = updated.(chatModel)

	if next.loading {
		t.Fatal("expected loading to stop after response")
	}
	if len(next.messages) != 2 {
		t.Fatalf("expected 2 messages after reply, got %d", len(next.messages))
	}
	if next.messages[1].Role != roleAssistant || next.messages[1].Content != "Hello back" {
		t.Fatalf("unexpected assistant message: %+v", next.messages[1])
	}
}

func TestSendIncludesPriorConversationHistory(t *testing.T) {
	t.Parallel()

	llm := &stubLLM{reply: "Final answer"}
	m := NewChatModel(llm)
	m.messages = []provider.Message{
		{Role: roleUser, Content: "First"},
		{Role: roleAssistant, Content: "Second"},
	}
	m.input = "Third"

	_, cmd := m.Update(enterKey())
	if cmd == nil {
		t.Fatal("expected async command")
	}
	_ = cmd()

	if len(llm.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(llm.calls))
	}
	got := llm.calls[0]
	if len(got) != 3 {
		t.Fatalf("expected 3 messages in history, got %d", len(got))
	}
	if got[0].Content != "First" || got[1].Content != "Second" || got[2].Content != "Third" {
		t.Fatalf("unexpected history order: %+v", got)
	}
}

func TestRequestFailureShowsErrorAndKeepsChatUsable(t *testing.T) {
	t.Parallel()

	llm := &stubLLM{err: errors.New("request failed")}
	m := NewChatModel(llm)
	m.input = "Hello"

	updated, cmd := m.Update(enterKey())
	if cmd == nil {
		t.Fatal("expected async command")
	}

	updated, _ = updated.(chatModel).Update(cmd())
	next := updated.(chatModel)

	if next.loading {
		t.Fatal("expected loading to stop after error")
	}
	if next.errMessage == "" {
		t.Fatal("expected ui error message")
	}
	if len(next.messages) != 1 {
		t.Fatalf("expected only the user message to remain, got %d", len(next.messages))
	}

	view := next.View().Content
	if !strings.Contains(view, "request failed") {
		t.Fatalf("expected error in view, got %q", view)
	}
	if !strings.Contains(view, "> ") {
		t.Fatalf("expected input area in view, got %q", view)
	}
}

func TestViewKeepsInputVisibleWhenHeightIsSmall(t *testing.T) {
	t.Parallel()

	m := NewChatModel(&stubLLM{})
	m.height = 6
	m.messages = []provider.Message{
		{Role: roleUser, Content: "one"},
		{Role: roleAssistant, Content: "two"},
		{Role: roleUser, Content: "three"},
		{Role: roleAssistant, Content: "four"},
	}
	m.input = "next"

	view := m.View().Content
	if !strings.Contains(view, "> next") {
		t.Fatalf("expected input to stay visible, got %q", view)
	}
	if strings.Contains(view, "one") {
		t.Fatalf("expected older lines to be trimmed in small view, got %q", view)
	}
}

type stubLLM struct {
	reply string
	err   error
	calls [][]provider.Message
}

func (s *stubLLM) Chat(_ context.Context, messages []provider.Message) (string, error) {
	copied := append([]provider.Message(nil), messages...)
	s.calls = append(s.calls, copied)
	if s.err != nil {
		return "", s.err
	}
	return s.reply, nil
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}
